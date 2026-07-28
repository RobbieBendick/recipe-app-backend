package recipeimport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const browserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"

var (
	igShortcodeRe = regexp.MustCompile(`(?i)/(?:reel|reels|p|tv)/([A-Za-z0-9_-]+)`)
	captionJSONRe = regexp.MustCompile(`(?i)"(?:caption|accessibility_caption)"\s*:\s*\{[^}]*?"text"\s*:\s*"((?:\\.|[^"\\])*)"`)
	captionTextRe = regexp.MustCompile(`(?i)"(?:caption_text|text)"\s*:\s*"((?:\\.|[^"\\])*)"`)
	edgeCaptionRe = regexp.MustCompile(`(?i)"edge_media_to_caption"\s*:\s*\{.*?"text"\s*:\s*"((?:\\.|[^"\\])*)"`)
	igTitleCapRe  = regexp.MustCompile(`(?is)^(?:.+?)\s+on\s+Instagram:\s*[“"'](.+?)[”"']\s*$`)
	igDescCapRe   = regexp.MustCompile(`(?is):\s*[“"'](.+?)[”"']\s*\.?\s*$`)
	engagementRe  = regexp.MustCompile(`(?i)^\d[\d.,KMB]*\s+(?:likes?|comments?|views?).+?:\s*[“"']?`)
)

type socialMeta struct {
	Platform string
	Title    string
	Caption  string
	HTML     string
}

func isSocialMediaURL(u *url.URL) bool {
	host := strings.ToLower(u.Hostname())
	host = strings.TrimPrefix(host, "www.")
	host = strings.TrimPrefix(host, "m.")
	switch host {
	case "instagram.com", "instagr.am", "facebook.com", "fb.com", "fb.watch":
		return true
	}
	return strings.HasSuffix(host, ".facebook.com")
}

func fromSocialURL(ctx context.Context, parsed *url.URL, opts Options) (*Extracted, error) {
	meta, err := fetchSocialMeta(ctx, parsed)
	if err != nil {
		return nil, err
	}

	caption := strings.TrimSpace(meta.Caption)
	title := strings.TrimSpace(meta.Title)
	if caption == "" && title == "" && strings.TrimSpace(meta.HTML) == "" {
		return nil, fmt.Errorf("%w: no caption found (is the post public?)", ErrNoRecipeData)
	}

	structured := &Extracted{
		Title:       title,
		Description: truncateRunes(caption, 500),
		Ingredients: []string{},
		Steps:       []string{},
		SourceURL:   parsed.String(),
	}

	textForAI := caption
	if textForAI == "" {
		textForAI = htmlToPlainText(meta.HTML)
	} else if meta.HTML != "" {
		// Prefer caption, but include a little page text for title context.
		textForAI = "TITLE: " + title + "\n\nCAPTION:\n" + caption
	} else {
		textForAI = "TITLE: " + title + "\n\nCAPTION:\n" + caption
	}

	var out *Extracted
	if strings.TrimSpace(opts.GeminiAPIKey) != "" {
		aiOut, aiErr := extractWithGeminiText(ctx, opts.GeminiAPIKey, opts.GeminiModel, parsed.String(), textForAI, true)
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

func fetchSocialMeta(ctx context.Context, parsed *url.URL) (*socialMeta, error) {
	host := strings.ToLower(parsed.Hostname())
	host = strings.TrimPrefix(host, "www.")
	host = strings.TrimPrefix(host, "m.")

	meta := &socialMeta{Platform: "social"}
	urls := []string{parsed.String()}

	if host == "instagram.com" || host == "instagr.am" {
		meta.Platform = "instagram"
		if embed := instagramEmbedURL(parsed); embed != "" {
			urls = append([]string{embed}, urls...)
		}
	} else {
		meta.Platform = "facebook"
	}

	var lastErr error
	for _, pageURL := range urls {
		html, err := fetchHTMLWithUA(ctx, pageURL, browserUserAgent)
		if err != nil {
			lastErr = err
			continue
		}
		meta.HTML = html
		title := firstMeta(html, ogTitleRe, ogTitleRe2)
		if title == "" {
			if m := htmlTitleRe.FindStringSubmatch(html); len(m) == 2 {
				title = cleanText(m[1])
			}
		}
		ogDesc := firstMeta(html, ogDescRe, ogDescRe2)
		caption := extractSocialCaption(html, title, ogDesc, meta.Platform)
		if caption != "" || title != "" {
			meta.Title = cleanSocialTitle(title, meta.Platform)
			meta.Caption = caption
			if meta.Caption == "" {
				meta.Caption = cleanSocialCaption(ogDesc, meta.Platform)
			}
			if meta.Caption != "" || meta.Title != "" {
				return meta, nil
			}
		}
		// Keep trying other URLs; remember whatever HTML we got.
		if meta.Title == "" {
			meta.Title = cleanSocialTitle(title, meta.Platform)
		}
		if meta.Caption == "" {
			meta.Caption = cleanSocialCaption(ogDesc, meta.Platform)
		}
	}

	if meta.Caption != "" || meta.Title != "" || meta.HTML != "" {
		return meta, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("%w: status blocked or empty", ErrFetchFailed)
}

func instagramEmbedURL(u *url.URL) string {
	m := igShortcodeRe.FindStringSubmatch(u.Path)
	if len(m) < 2 {
		return ""
	}
	code := m[1]
	kind := "p"
	lower := strings.ToLower(u.Path)
	if strings.Contains(lower, "/reel") || strings.Contains(lower, "/tv/") {
		kind = "reel"
	}
	return fmt.Sprintf("https://www.instagram.com/%s/%s/embed/captioned/", kind, code)
}

func extractSocialCaption(pageHTML, ogTitle, ogDesc, platform string) string {
	candidates := make([]string, 0, 8)
	for _, re := range []*regexp.Regexp{captionJSONRe, edgeCaptionRe, captionTextRe} {
		for _, m := range re.FindAllStringSubmatch(pageHTML, -1) {
			if len(m) < 2 {
				continue
			}
			if text := decodeJSONString(m[1]); text != "" {
				candidates = append(candidates, text)
			}
		}
	}
	if cap := captionFromOGTitle(ogTitle); cap != "" {
		candidates = append(candidates, cap)
	}
	if cap := cleanSocialCaption(ogDesc, platform); cap != "" {
		candidates = append(candidates, cap)
	}

	best := ""
	for _, c := range candidates {
		c = cleanText(c)
		if len(c) > len(best) {
			best = c
		}
	}
	return best
}

func captionFromOGTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	if m := igTitleCapRe.FindStringSubmatch(title); len(m) == 2 {
		return cleanText(m[1])
	}
	// Facebook often uses: Name - caption...
	if i := strings.Index(title, " - "); i > 0 && i+3 < len(title) {
		rest := strings.TrimSpace(title[i+3:])
		if len(rest) > 20 {
			return cleanText(rest)
		}
	}
	return ""
}

func cleanSocialTitle(title, platform string) string {
	title = cleanText(title)
	if title == "" {
		return ""
	}
	if m := igTitleCapRe.FindStringSubmatch(title); len(m) == 2 {
		cap := cleanText(m[1])
		// Use a short title from the first line of caption if possible.
		if line := firstLine(cap); line != "" && len(line) <= 80 {
			return line
		}
		return truncateRunes(cap, 80)
	}
	if platform == "facebook" {
		if i := strings.Index(title, " | "); i > 0 {
			title = strings.TrimSpace(title[:i])
		}
	}
	return title
}

func cleanSocialCaption(desc, platform string) string {
	desc = cleanText(desc)
	if desc == "" {
		return ""
	}
	if m := igDescCapRe.FindStringSubmatch(desc); len(m) == 2 {
		return cleanText(m[1])
	}
	desc = engagementRe.ReplaceAllString(desc, "")
	desc = strings.TrimSpace(desc)
	desc = strings.Trim(desc, `"'“”`)
	if platform == "instagram" && strings.Contains(strings.ToLower(desc), " likes") && len(desc) < 40 {
		return ""
	}
	return desc
}

func decodeJSONString(escaped string) string {
	var s string
	if err := json.Unmarshal([]byte(`"`+escaped+`"`), &s); err != nil {
		return cleanText(strings.ReplaceAll(escaped, `\n`, "\n"))
	}
	return cleanText(s)
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if i := strings.IndexAny(s, "\n\r"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	return s
}
