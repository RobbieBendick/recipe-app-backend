package recipeimport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	maxBodyBytes     = 2 << 20 // 2 MiB
	fetchTimeout     = 15 * time.Second
	defaultUserAgent = "RecipeAppBot/1.0 (+https://github.com/robbi/recipe-app)"
)

var (
	ErrInvalidURL   = errors.New("invalid URL")
	ErrBlockedHost  = errors.New("URL host is not allowed")
	ErrFetchFailed  = errors.New("failed to fetch URL")
	ErrNoRecipeData = errors.New("no recipe data found on page")
	ErrAIFailed     = errors.New("AI extraction failed")
)

// Extracted is a best-effort recipe parsed from a web page.
// Empty / zero fields mean the source did not provide that data.
type Extracted struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Ingredients []string `json:"ingredients"`
	Steps       []string `json:"steps"`
	PrepMinutes int      `json:"prepMinutes"`
	CookMinutes int      `json:"cookMinutes"`
	Servings    int      `json:"servings"`
	SourceURL   string   `json:"sourceUrl"`
}

// Options controls optional AI-assisted extraction.
type Options struct {
	GeminiAPIKey string
	GeminiModel  string
}

var (
	ldJSONScriptRe = regexp.MustCompile(`(?is)<script[^>]*type\s*=\s*["']application/ld\+json["'][^>]*>(.*?)</script>`)
	ogTitleRe      = regexp.MustCompile(`(?is)<meta[^>]+property\s*=\s*["']og:title["'][^>]+content\s*=\s*["']([^"']+)["']`)
	ogTitleRe2     = regexp.MustCompile(`(?is)<meta[^>]+content\s*=\s*["']([^"']+)["'][^>]+property\s*=\s*["']og:title["']`)
	ogDescRe       = regexp.MustCompile(`(?is)<meta[^>]+property\s*=\s*["']og:description["'][^>]+content\s*=\s*["']([^"']+)["']`)
	ogDescRe2      = regexp.MustCompile(`(?is)<meta[^>]+content\s*=\s*["']([^"']+)["'][^>]+property\s*=\s*["']og:description["']`)
	htmlTitleRe    = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	isoDurationRe  = regexp.MustCompile(`(?i)^P(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?)?$`)
	yieldNumberRe  = regexp.MustCompile(`(\d+(?:\.\d+)?)`)
	htmlTagRe      = regexp.MustCompile(`(?s)<[^>]+>`)
)

// FromURL fetches the page and extracts whatever recipe fields it can.
// When a Gemini API key is provided, AI extraction is preferred and JSON-LD fills gaps.
func FromURL(ctx context.Context, rawURL string, opts Options) (*Extracted, error) {
	parsed, err := validateURL(rawURL)
	if err != nil {
		return nil, err
	}

	html, err := fetchHTML(ctx, parsed.String())
	if err != nil {
		return nil, err
	}

	structured := &Extracted{
		Ingredients: []string{},
		Steps:       []string{},
		SourceURL:   parsed.String(),
	}
	if recipe := findRecipeJSONLD(html); recipe != nil {
		applyJSONLD(structured, recipe)
	}
	if structured.Title == "" {
		structured.Title = firstMeta(html, ogTitleRe, ogTitleRe2)
	}
	if structured.Title == "" {
		if m := htmlTitleRe.FindStringSubmatch(html); len(m) == 2 {
			structured.Title = cleanText(m[1])
		}
	}
	if structured.Description == "" {
		structured.Description = firstMeta(html, ogDescRe, ogDescRe2)
	}

	var out *Extracted
	if strings.TrimSpace(opts.GeminiAPIKey) != "" {
		aiOut, aiErr := extractWithGemini(ctx, opts.GeminiAPIKey, opts.GeminiModel, parsed.String(), html)
		if aiErr == nil && hasUsefulRecipe(aiOut) {
			out = aiOut
			fillGaps(out, structured)
		} else if hasUsefulRecipe(structured) {
			out = structured
		} else if aiErr != nil {
			return nil, fmt.Errorf("%w: %v", ErrAIFailed, aiErr)
		}
	} else {
		out = structured
	}

	if !hasUsefulRecipe(out) {
		return nil, ErrNoRecipeData
	}
	if out.Ingredients == nil {
		out.Ingredients = []string{}
	}
	if out.Steps == nil {
		out.Steps = []string{}
	}
	out.SourceURL = parsed.String()
	return out, nil
}

func fillGaps(dst, src *Extracted) {
	if dst == nil || src == nil {
		return
	}
	if dst.Title == "" {
		dst.Title = src.Title
	}
	if dst.Description == "" {
		dst.Description = src.Description
	}
	if len(dst.Ingredients) == 0 && len(src.Ingredients) > 0 {
		dst.Ingredients = src.Ingredients
	}
	if len(dst.Steps) == 0 && len(src.Steps) > 0 {
		dst.Steps = src.Steps
	}
	if dst.PrepMinutes == 0 {
		dst.PrepMinutes = src.PrepMinutes
	}
	if dst.CookMinutes == 0 {
		dst.CookMinutes = src.CookMinutes
	}
	if dst.Servings == 0 {
		dst.Servings = src.Servings
	}
}

func validateURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, ErrInvalidURL
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return nil, ErrInvalidURL
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, ErrInvalidURL
	}
	host := parsed.Hostname()
	if host == "" {
		return nil, ErrInvalidURL
	}
	if isBlockedHost(host) {
		return nil, ErrBlockedHost
	}
	return parsed, nil
}

func isBlockedHost(host string) bool {
	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") || lower == "metadata.google.internal" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		addrs, err := net.LookupIP(host)
		if err != nil {
			return false
		}
		for _, addr := range addrs {
			if isPrivateIP(addr) {
				return true
			}
		}
		return false
	}
	return isPrivateIP(ip)
}

func isPrivateIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}

func fetchHTML(ctx context.Context, pageURL string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return "", ErrFetchFailed
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.8")

	client := &http.Client{
		Timeout: fetchTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			if err := validateRedirect(req.URL); err != nil {
				return err
			}
			return nil
		},
	}

	res, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrFetchFailed, err)
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("%w: status %d", ErrFetchFailed, res.StatusCode)
	}

	limited := io.LimitReader(res.Body, maxBodyBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrFetchFailed, err)
	}
	if len(body) > maxBodyBytes {
		body = body[:maxBodyBytes]
	}
	return string(body), nil
}

func validateRedirect(u *url.URL) error {
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return ErrInvalidURL
	}
	if isBlockedHost(u.Hostname()) {
		return ErrBlockedHost
	}
	return nil
}

func findRecipeJSONLD(html string) map[string]any {
	matches := ldJSONScriptRe.FindAllStringSubmatch(html, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		raw := strings.TrimSpace(match[1])
		if raw == "" {
			continue
		}
		var decoded any
		if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
			continue
		}
		if recipe := findRecipeNode(decoded); recipe != nil {
			return recipe
		}
	}
	return nil
}

func findRecipeNode(node any) map[string]any {
	switch v := node.(type) {
	case map[string]any:
		if hasType(v, "Recipe") {
			return v
		}
		if graph, ok := v["@graph"]; ok {
			if recipe := findRecipeNode(graph); recipe != nil {
				return recipe
			}
		}
		if main, ok := v["mainEntity"]; ok {
			if recipe := findRecipeNode(main); recipe != nil {
				return recipe
			}
		}
	case []any:
		for _, item := range v {
			if recipe := findRecipeNode(item); recipe != nil {
				return recipe
			}
		}
	}
	return nil
}

func hasType(obj map[string]any, want string) bool {
	raw, ok := obj["@type"]
	if !ok {
		return false
	}
	switch t := raw.(type) {
	case string:
		return strings.EqualFold(t, want)
	case []any:
		for _, item := range t {
			if s, ok := item.(string); ok && strings.EqualFold(s, want) {
				return true
			}
		}
	}
	return false
}

func applyJSONLD(out *Extracted, recipe map[string]any) {
	if name := asString(recipe["name"]); name != "" {
		out.Title = name
	}
	if desc := asString(recipe["description"]); desc != "" {
		out.Description = desc
	}
	if ingredients := asStringList(recipe["recipeIngredient"]); len(ingredients) > 0 {
		out.Ingredients = ingredients
	}
	if steps := parseInstructions(recipe["recipeInstructions"]); len(steps) > 0 {
		out.Steps = steps
	}
	if mins := parseISODurationMinutes(recipe["prepTime"]); mins > 0 {
		out.PrepMinutes = mins
	}
	if mins := parseISODurationMinutes(recipe["cookTime"]); mins > 0 {
		out.CookMinutes = mins
	}
	if servings := parseYield(recipe["recipeYield"]); servings > 0 {
		out.Servings = servings
	}
}

func parseInstructions(raw any) []string {
	switch v := raw.(type) {
	case string:
		return nonEmptyLines(v)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, parseInstructions(item)...)
		}
		return out
	case map[string]any:
		if hasType(v, "HowToSection") {
			return parseInstructions(v["itemListElement"])
		}
		if text := asString(v["text"]); text != "" {
			return []string{cleanText(text)}
		}
		if name := asString(v["name"]); name != "" {
			return []string{cleanText(name)}
		}
		return parseInstructions(v["itemListElement"])
	default:
		return nil
	}
}

func parseISODurationMinutes(raw any) int {
	s := asString(raw)
	if s == "" {
		return 0
	}
	m := isoDurationRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0
	}
	days := atoi(m[1])
	hours := atoi(m[2])
	mins := atoi(m[3])
	secs := atoi(m[4])
	total := days*24*60 + hours*60 + mins
	if secs >= 30 {
		total++
	}
	return total
}

func parseYield(raw any) int {
	switch v := raw.(type) {
	case float64:
		if v > 0 {
			return int(v + 0.5)
		}
	case string:
		if n := yieldNumberRe.FindString(v); n != "" {
			f, err := strconv.ParseFloat(n, 64)
			if err == nil && f > 0 {
				return int(f + 0.5)
			}
		}
	case []any:
		for _, item := range v {
			if n := parseYield(item); n > 0 {
				return n
			}
		}
	}
	return 0
}

func asString(raw any) string {
	switch v := raw.(type) {
	case string:
		return cleanText(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if s := asString(item); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, " ")
	default:
		return ""
	}
}

func asStringList(raw any) []string {
	switch v := raw.(type) {
	case string:
		return nonEmptyLines(v)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s := asString(item); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func nonEmptyLines(text string) []string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = cleanText(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func cleanText(s string) string {
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = htmlTagRe.ReplaceAllString(s, " ")
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimSpace(s)
}

func firstMeta(html string, patterns ...*regexp.Regexp) string {
	for _, re := range patterns {
		if m := re.FindStringSubmatch(html); len(m) == 2 {
			return cleanText(m[1])
		}
	}
	return ""
}

func atoi(s string) int {
	if s == "" {
		return 0
	}
	n, _ := strconv.Atoi(s)
	return n
}
