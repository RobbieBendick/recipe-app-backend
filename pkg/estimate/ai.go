package estimate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	aiTimeout      = 45 * time.Second
	aiMaxCandidates = 12
)

// Assist uses Gemini to improve Kroger search terms and product picks.
type Assist struct {
	APIKey string
	Model  string

	mu          sync.Mutex
	searchCache map[string]searchHint
}

type searchHint struct {
	SearchTerm string   `json:"searchTerm"`
	Prefer     []string `json:"prefer"`
	Exclude    []string `json:"exclude"`
}

type productCandidate struct {
	ProductID   string  `json:"productId"`
	Description string  `json:"description"`
	Brand       string  `json:"brand,omitempty"`
	Size        string  `json:"size,omitempty"`
	Price       float64 `json:"price,omitempty"`
}

type productChoiceAsk struct {
	Key         string
	Ingredient  string
	SearchTerm  string
	Candidates  []productCandidate
}

func NewAssist(apiKey, model string) *Assist {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = "gemini-2.0-flash"
	}
	return &Assist{
		APIKey:      apiKey,
		Model:       model,
		searchCache: map[string]searchHint{},
	}
}

func (a *Assist) Enabled() bool {
	return a != nil && strings.TrimSpace(a.APIKey) != ""
}

func (a *Assist) hintFor(name string) (searchHint, bool) {
	if !a.Enabled() {
		return searchHint{}, false
	}
	key := normalizeName(name)
	a.mu.Lock()
	defer a.mu.Unlock()
	h, ok := a.searchCache[key]
	return h, ok
}

// WarmSearchHints asks Gemini for Kroger search queries for unique ingredient names.
// Fallback terms are used when AI fails or omits an item.
func (a *Assist) WarmSearchHints(ctx context.Context, names []string, fallbacks map[string]string) {
	if !a.Enabled() || len(names) == 0 {
		return
	}

	uniq := make([]string, 0, len(names))
	seen := map[string]bool{}
	missing := make([]string, 0)
	a.mu.Lock()
	for _, name := range names {
		key := normalizeName(name)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		uniq = append(uniq, key)
		if _, ok := a.searchCache[key]; !ok {
			missing = append(missing, key)
		}
	}
	a.mu.Unlock()
	if len(missing) == 0 {
		return
	}

	type askItem struct {
		Name     string `json:"name"`
		Fallback string `json:"fallbackSearch"`
	}
	asks := make([]askItem, 0, len(missing))
	for _, name := range missing {
		asks = append(asks, askItem{
			Name:     name,
			Fallback: strings.TrimSpace(fallbacks[name]),
		})
	}

	var prompt strings.Builder
	prompt.WriteString("You help match recipe ingredients to grocery products at Kroger.\n")
	prompt.WriteString("For each ingredient, choose a short Kroger product search query that finds the correct pantry/grocery item.\n")
	prompt.WriteString("searchTerm MUST be at most 5 plain words (Kroger rejects longer queries).\n")
	prompt.WriteString("Prefer the core grocery product (e.g. \"black beans\", \"chicken breast\", \"flour tortillas\").\n")
	prompt.WriteString("Drop prep words like rinsed, drained, diced into, boneless notes unless needed to identify the product.\n")
	prompt.WriteString("Avoid desserts, candy, ice cream, prepared meals, and unrelated products when the ingredient is a cooking staple.\n")
	prompt.WriteString("prefer: words that should appear in a good product title.\n")
	prompt.WriteString("exclude: words that indicate a wrong product.\n")
	prompt.WriteString("Return one item per input name.\n\n")
	rawAsks, _ := json.Marshal(asks)
	prompt.WriteString(string(rawAsks))

	schema := map[string]any{
		"type": "OBJECT",
		"properties": map[string]any{
			"items": map[string]any{
				"type": "ARRAY",
				"items": map[string]any{
					"type": "OBJECT",
					"properties": map[string]any{
						"name":       map[string]any{"type": "STRING"},
						"searchTerm": map[string]any{"type": "STRING"},
						"prefer": map[string]any{
							"type":  "ARRAY",
							"items": map[string]any{"type": "STRING"},
						},
						"exclude": map[string]any{
							"type":  "ARRAY",
							"items": map[string]any{"type": "STRING"},
						},
					},
					"required": []string{"name", "searchTerm", "prefer", "exclude"},
				},
			},
		},
		"required": []string{"items"},
	}

	var parsed struct {
		Items []struct {
			Name       string   `json:"name"`
			SearchTerm string   `json:"searchTerm"`
			Prefer     []string `json:"prefer"`
			Exclude    []string `json:"exclude"`
		} `json:"items"`
	}
	if err := a.generateJSON(ctx, prompt.String(), schema, &parsed); err != nil {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	for _, item := range parsed.Items {
		key := normalizeName(item.Name)
		if key == "" {
			continue
		}
		term := strings.TrimSpace(item.SearchTerm)
		if len(term) < 3 {
			term = strings.TrimSpace(fallbacks[key])
		}
		term = ClampSearchTerm(term, 5)
		if term == "" {
			continue
		}
		a.searchCache[key] = searchHint{
			SearchTerm: term,
			Prefer:     cleanTokenList(item.Prefer),
			Exclude:    cleanTokenList(item.Exclude),
		}
	}
}

// ChooseProducts asks Gemini which candidate best matches each ingredient.
// Returns map[ingredientKey]productId.
func (a *Assist) ChooseProducts(ctx context.Context, asks []productChoiceAsk) map[string]string {
	out := map[string]string{}
	if !a.Enabled() || len(asks) == 0 {
		return out
	}

	type cand struct {
		ProductID   string  `json:"productId"`
		Description string  `json:"description"`
		Brand       string  `json:"brand,omitempty"`
		Size        string  `json:"size,omitempty"`
		Price       float64 `json:"price,omitempty"`
	}
	type ask struct {
		Key        string `json:"key"`
		Ingredient string `json:"ingredient"`
		SearchTerm string `json:"searchTerm"`
		Candidates []cand `json:"candidates"`
	}

	payload := make([]ask, 0, len(asks))
	for _, item := range asks {
		if strings.TrimSpace(item.Key) == "" || len(item.Candidates) == 0 {
			continue
		}
		cands := make([]cand, 0, len(item.Candidates))
		for i, c := range item.Candidates {
			if i >= aiMaxCandidates {
				break
			}
			if strings.TrimSpace(c.ProductID) == "" {
				continue
			}
			cands = append(cands, cand{
				ProductID:   c.ProductID,
				Description: c.Description,
				Brand:       c.Brand,
				Size:        c.Size,
				Price:       c.Price,
			})
		}
		if len(cands) == 0 {
			continue
		}
		payload = append(payload, ask{
			Key:        item.Key,
			Ingredient: item.Ingredient,
			SearchTerm: item.SearchTerm,
			Candidates: cands,
		})
	}
	if len(payload) == 0 {
		return out
	}

	var prompt strings.Builder
	prompt.WriteString("Pick the best Kroger product for each recipe ingredient.\n")
	prompt.WriteString("Choose the product that a home cook would buy for that ingredient in a normal recipe.\n")
	prompt.WriteString("Reject candy, ice cream, meals, baklava, candles, and other wrong categories.\n")
	prompt.WriteString("You MUST choose productId from the provided candidates only.\n")
	prompt.WriteString("If none are appropriate, omit that item.\n\n")
	rawAsks, _ := json.Marshal(payload)
	prompt.WriteString(string(rawAsks))

	schema := map[string]any{
		"type": "OBJECT",
		"properties": map[string]any{
			"picks": map[string]any{
				"type": "ARRAY",
				"items": map[string]any{
					"type": "OBJECT",
					"properties": map[string]any{
						"key":       map[string]any{"type": "STRING"},
						"productId": map[string]any{"type": "STRING"},
					},
					"required": []string{"key", "productId"},
				},
			},
		},
		"required": []string{"picks"},
	}

	var parsed struct {
		Picks []struct {
			Key       string `json:"key"`
			ProductID string `json:"productId"`
		} `json:"picks"`
	}
	if err := a.generateJSON(ctx, prompt.String(), schema, &parsed); err != nil {
		return out
	}

	allowed := map[string]map[string]bool{}
	for _, item := range payload {
		set := map[string]bool{}
		for _, c := range item.Candidates {
			set[c.ProductID] = true
		}
		allowed[item.Key] = set
	}
	for _, pick := range parsed.Picks {
		key := strings.TrimSpace(pick.Key)
		id := strings.TrimSpace(pick.ProductID)
		if key == "" || id == "" {
			continue
		}
		if set := allowed[key]; set != nil && set[id] {
			out[key] = id
		}
	}
	return out
}

func (a *Assist) resolveSearchTerm(name, fallback string) (term string, prefer, exclude []string) {
	term = ClampSearchTerm(strings.TrimSpace(fallback), 8)
	if h, ok := a.hintFor(name); ok {
		if strings.TrimSpace(h.SearchTerm) != "" {
			term = ClampSearchTerm(strings.TrimSpace(h.SearchTerm), 5)
		}
		prefer = h.Prefer
		exclude = h.Exclude
	}
	return term, prefer, exclude
}

func (a *Assist) generateJSON(ctx context.Context, prompt string, schema map[string]any, dest any) error {
	endpoint := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		a.Model,
		a.APIKey,
	)
	body, err := json.Marshal(map[string]any{
		"contents": []map[string]any{{
			"parts": []map[string]any{{"text": prompt}},
		}},
		"generationConfig": map[string]any{
			"temperature":      0.1,
			"responseMimeType": "application/json",
			"responseSchema":   schema,
		},
	})
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, aiTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return err
	}

	var envelope struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return err
	}
	if envelope.Error != nil && envelope.Error.Message != "" {
		return fmt.Errorf("gemini: %s", envelope.Error.Message)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("gemini status %d", res.StatusCode)
	}
	if len(envelope.Candidates) == 0 || len(envelope.Candidates[0].Content.Parts) == 0 {
		return fmt.Errorf("gemini returned no content")
	}

	text := strings.TrimSpace(envelope.Candidates[0].Content.Parts[0].Text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)
	return json.Unmarshal([]byte(text), dest)
}

func cleanTokenList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, v := range values {
		v = strings.ToLower(strings.TrimSpace(v))
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
