package recipeimport

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	maxShoppingImageBytes = 3_500_000
	maxShoppingListItems  = 80
)

// ShoppingListExtract is grocery items read from a photo.
type ShoppingListExtract struct {
	Title string   `json:"title"`
	Items []string `json:"items"`
}

type geminiShoppingJSON struct {
	Title string   `json:"title"`
	Items []string `json:"items"`
}

var allowedImageMIMEs = map[string]string{
	"image/jpeg": "image/jpeg",
	"image/jpg":  "image/jpeg",
	"image/png":  "image/png",
	"image/webp": "image/webp",
	"image/gif":  "image/gif",
	"image/heic": "image/heic",
	"image/heif": "image/heif",
}

// FromImage asks Gemini to read shopping items from a photo (handwritten list,
// printed list, receipt, or physical groceries).
func FromImage(ctx context.Context, apiKey, model, mimeType, imageBase64 string) (*ShoppingListExtract, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, fmt.Errorf("%w: missing gemini api key", ErrAIFailed)
	}

	data, mime, err := decodeImagePayload(imageBase64, mimeType)
	if err != nil {
		return nil, err
	}

	model = strings.TrimSpace(model)
	if model == "" {
		model = "gemini-2.0-flash"
	}

	endpoint := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		model,
		apiKey,
	)

	payload := geminiRequest{
		Contents: []geminiContent{{
			Parts: []geminiPart{
				{Text: shoppingImagePrompt()},
				{InlineData: &geminiInlineData{
					MimeType: mime,
					Data:     base64.StdEncoding.EncodeToString(data),
				}},
			},
		}},
		GenerationConfig: geminiGenerationConfig{
			Temperature:      0.1,
			ResponseMIMEType: "application/json",
			ResponseSchema: map[string]any{
				"type": "OBJECT",
				"properties": map[string]any{
					"title": map[string]any{"type": "STRING"},
					"items": map[string]any{
						"type":  "ARRAY",
						"items": map[string]any{"type": "STRING"},
					},
				},
				"required": []string{"title", "items"},
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
		return nil, fmt.Errorf("%w: %v", ErrAIFailed, err)
	}
	defer res.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAIFailed, err)
	}

	var parsed geminiResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("%w: invalid gemini response", ErrAIFailed)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return nil, fmt.Errorf("%w: %s", ErrAIFailed, parsed.Error.Message)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: gemini status %d", ErrAIFailed, res.StatusCode)
	}
	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("%w: gemini returned no content", ErrAIFailed)
	}

	text := strings.TrimSpace(parsed.Candidates[0].Content.Parts[0].Text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	var parsedList geminiShoppingJSON
	if err := json.Unmarshal([]byte(text), &parsedList); err != nil {
		return nil, fmt.Errorf("%w: gemini json parse: %v", ErrAIFailed, err)
	}

	items := normalizeStringList(parsedList.Items)
	if len(items) > maxShoppingListItems {
		items = items[:maxShoppingListItems]
	}
	if len(items) == 0 {
		return nil, ErrNoListItems
	}

	return &ShoppingListExtract{
		Title: cleanText(parsedList.Title),
		Items: items,
	}, nil
}

func shoppingImagePrompt() string {
	var b strings.Builder
	b.WriteString("Look at this photo and extract a shopping list.\n")
	b.WriteString("The photo may be a handwritten or printed grocery list, a receipt, notes on a phone or whiteboard, or actual groceries / pantry items.\n")
	b.WriteString("Return grocery items a shopper would buy. One item per array entry, including amounts when clearly shown.\n")
	b.WriteString("Do not invent items that are not visible.\n")
	b.WriteString("Skip prices, store names, ads, and decorative text unless they are product names.\n")
	b.WriteString("If a heading or list title is visible, put it in title; otherwise use an empty string.\n")
	b.WriteString("If nothing grocery-related is visible, return an empty items array.\n")
	return b.String()
}

func decodeImagePayload(imageBase64, mimeType string) ([]byte, string, error) {
	mime, ok := allowedImageMIMEs[strings.ToLower(strings.TrimSpace(mimeType))]
	if !ok {
		return nil, "", fmt.Errorf("%w: unsupported image type", ErrInvalidImage)
	}

	raw := strings.TrimSpace(imageBase64)
	if raw == "" {
		return nil, "", fmt.Errorf("%w: missing image data", ErrInvalidImage)
	}
	if i := strings.Index(raw, ","); i >= 0 && strings.Contains(strings.ToLower(raw[:i]), "base64") {
		raw = raw[i+1:]
	}
	raw = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, raw)

	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		data, err = base64.RawStdEncoding.DecodeString(raw)
	}
	if err != nil {
		return nil, "", fmt.Errorf("%w: invalid image data", ErrInvalidImage)
	}
	if len(data) == 0 {
		return nil, "", fmt.Errorf("%w: empty image", ErrInvalidImage)
	}
	if len(data) > maxShoppingImageBytes {
		return nil, "", fmt.Errorf("%w: image is too large", ErrInvalidImage)
	}
	return data, mime, nil
}
