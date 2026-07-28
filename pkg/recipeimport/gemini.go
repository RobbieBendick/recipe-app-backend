package recipeimport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	geminiTimeout   = 45 * time.Second
	maxPageTextRunes = 48000
)

var (
	stripScriptRe   = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	stripStyleRe    = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	stripNoscriptRe = regexp.MustCompile(`(?is)<noscript[^>]*>.*?</noscript>`)
	stripSVGRe      = regexp.MustCompile(`(?is)<svg[^>]*>.*?</svg>`)
	stripIFrameRe   = regexp.MustCompile(`(?is)<iframe[^>]*>.*?</iframe>`)
	stripTagsRe     = regexp.MustCompile(`(?is)<[^>]+>`)
	multiSpaceRe    = regexp.MustCompile(`[ \t\x0a\x0d]{2,}`)
)

type geminiRequest struct {
	Contents         []geminiContent        `json:"contents"`
	GenerationConfig geminiGenerationConfig `json:"generationConfig"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiGenerationConfig struct {
	Temperature      float64        `json:"temperature"`
	ResponseMIMEType string         `json:"responseMimeType"`
	ResponseSchema   map[string]any `json:"responseSchema"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

type geminiRecipeJSON struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Ingredients []string `json:"ingredients"`
	Steps       []string `json:"steps"`
	PrepMinutes int      `json:"prepMinutes"`
	CookMinutes int      `json:"cookMinutes"`
	Servings    int      `json:"servings"`
}

func extractWithGemini(ctx context.Context, apiKey, model, pageURL, html string) (*Extracted, error) {
	pageText := htmlToPlainText(html)
	if pageText == "" {
		return nil, fmt.Errorf("page had no readable text")
	}
	return extractWithGeminiText(ctx, apiKey, model, pageURL, pageText, false)
}

func extractWithGeminiText(ctx context.Context, apiKey, model, pageURL, pageText string, socialCaption bool) (*Extracted, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, fmt.Errorf("missing gemini api key")
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = "gemini-2.0-flash"
	}
	pageText = strings.TrimSpace(pageText)
	if pageText == "" {
		return nil, fmt.Errorf("page had no readable text")
	}
	pageText = truncateRunes(pageText, maxPageTextRunes)

	prompt := buildGeminiPrompt(pageURL, pageText)
	if socialCaption {
		prompt = buildSocialGeminiPrompt(pageURL, pageText)
	}
	endpoint := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		model,
		apiKey,
	)

	payload := geminiRequest{
		Contents: []geminiContent{{
			Parts: []geminiPart{{Text: prompt}},
		}},
		GenerationConfig: geminiGenerationConfig{
			Temperature:      0.1,
			ResponseMIMEType: "application/json",
			ResponseSchema: map[string]any{
				"type": "OBJECT",
				"properties": map[string]any{
					"title":       map[string]any{"type": "STRING"},
					"description": map[string]any{"type": "STRING"},
					"ingredients": map[string]any{
						"type":  "ARRAY",
						"items": map[string]any{"type": "STRING"},
					},
					"steps": map[string]any{
						"type":  "ARRAY",
						"items": map[string]any{"type": "STRING"},
					},
					"prepMinutes": map[string]any{"type": "INTEGER"},
					"cookMinutes": map[string]any{"type": "INTEGER"},
					"servings":    map[string]any{"type": "INTEGER"},
				},
				"required": []string{"title", "description", "ingredients", "steps", "prepMinutes", "cookMinutes", "servings"},
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, geminiTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	var parsed geminiResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("invalid gemini response: %w", err)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return nil, fmt.Errorf("gemini: %s", parsed.Error.Message)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("gemini status %d", res.StatusCode)
	}
	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("gemini returned no content")
	}

	text := strings.TrimSpace(parsed.Candidates[0].Content.Parts[0].Text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	var recipe geminiRecipeJSON
	if err := json.Unmarshal([]byte(text), &recipe); err != nil {
		return nil, fmt.Errorf("gemini json parse: %w", err)
	}

	out := &Extracted{
		Title:       cleanText(recipe.Title),
		Description: cleanText(recipe.Description),
		Ingredients: normalizeStringList(recipe.Ingredients),
		Steps:       normalizeStringList(recipe.Steps),
		PrepMinutes: clampNonNeg(recipe.PrepMinutes),
		CookMinutes: clampNonNeg(recipe.CookMinutes),
		Servings:    clampNonNeg(recipe.Servings),
		SourceURL:   pageURL,
	}
	if !hasUsefulRecipe(out) {
		return nil, ErrNoRecipeData
	}
	return out, nil
}

func buildGeminiPrompt(pageURL, pageText string) string {
	var b strings.Builder
	b.WriteString("Extract the FIRST complete cooking recipe from this web page.\n")
	b.WriteString("Return only facts clearly present on the page. Do not invent ingredients or steps.\n")
	b.WriteString("Ignore ads, navigation, comments, related recipes, newsletter prompts, and affiliate fluff.\n")
	b.WriteString("If a field is unknown, use an empty string, empty array, or 0.\n")
	b.WriteString("ingredients: one ingredient per array item, including amounts when shown.\n")
	b.WriteString("steps: one cooking instruction per array item, in order. No step numbers needed.\n")
	b.WriteString("prepMinutes/cookMinutes/servings: integers only; 0 if unknown.\n\n")
	b.WriteString("Page URL: ")
	b.WriteString(pageURL)
	b.WriteString("\n\nPage text:\n")
	b.WriteString(pageText)
	return b.String()
}

func buildSocialGeminiPrompt(pageURL, captionText string) string {
	var b strings.Builder
	b.WriteString("Extract a cooking recipe from this Instagram/Facebook Reel or post caption/description.\n")
	b.WriteString("Focus on the CAPTION text. Recipe creators often list ingredients and steps in the description.\n")
	b.WriteString("Return only facts clearly present. Do not invent ingredients or steps that are not written.\n")
	b.WriteString("Ignore hashtags, @mentions, emojis-as-filler, and promotional links unless they are part of an ingredient name.\n")
	b.WriteString("If ingredients/steps are written as one block, split them into separate array items.\n")
	b.WriteString("If a field is unknown, use an empty string, empty array, or 0.\n")
	b.WriteString("ingredients: one ingredient per array item, including amounts when shown.\n")
	b.WriteString("steps: one cooking instruction per array item, in order.\n")
	b.WriteString("prepMinutes/cookMinutes/servings: integers only; 0 if unknown.\n\n")
	b.WriteString("Post URL: ")
	b.WriteString(pageURL)
	b.WriteString("\n\nCaption / description:\n")
	b.WriteString(captionText)
	return b.String()
}

func htmlToPlainText(rawHTML string) string {
	text := stripScriptRe.ReplaceAllString(rawHTML, " ")
	text = stripStyleRe.ReplaceAllString(text, " ")
	text = stripNoscriptRe.ReplaceAllString(text, " ")
	text = stripSVGRe.ReplaceAllString(text, " ")
	text = stripIFrameRe.ReplaceAllString(text, " ")
	text = stripTagsRe.ReplaceAllString(text, " ")
	text = html.UnescapeString(text)
	text = strings.ReplaceAll(text, "\u00a0", " ")
	text = multiSpaceRe.ReplaceAllString(text, " ")
	text = strings.TrimSpace(text)
	return truncateRunes(text, maxPageTextRunes)
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}

func normalizeStringList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = cleanText(v)
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	return out
}

func clampNonNeg(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

func hasUsefulRecipe(e *Extracted) bool {
	if e == nil {
		return false
	}
	return e.Title != "" || len(e.Ingredients) > 0 || len(e.Steps) > 0
}
