package kroger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	baseURL   = "https://api.kroger.com/v1"
	tokenURL  = "https://api.kroger.com/v1/connect/oauth2/token"
	tokenScope = "product.compact"
	cacheTTL  = 15 * time.Minute
)

type Client struct {
	clientID     string
	clientSecret string
	http         *http.Client

	mu          sync.Mutex
	accessToken string
	tokenExpiry time.Time

	cacheMu sync.Mutex
	cache   map[string]cacheEntry
}

type cacheEntry struct {
	at   time.Time
	data []Product
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

type Location struct {
	LocationID string `json:"locationId"`
	Name       string `json:"name"`
	Chain      string `json:"chain"`
	Address    struct {
		AddressLine1 string `json:"addressLine1"`
		City         string `json:"city"`
		State        string `json:"state"`
		ZipCode      string `json:"zipCode"`
	} `json:"address"`
}

type Product struct {
	ProductID   string `json:"productId"`
	UPC         string `json:"upc"`
	Description string `json:"description"`
	Brand       string `json:"brand"`
	Items       []struct {
		ItemID string `json:"itemId"`
		Size   string `json:"size"`
		Price  *struct {
			Regular float64 `json:"regular"`
			Promo   float64 `json:"promo"`
		} `json:"price"`
	} `json:"items"`
}

func NewClient(clientID, clientSecret string) *Client {
	return &Client{
		clientID:     cleanCredential(clientID),
		clientSecret: cleanCredential(clientSecret),
		http:         &http.Client{Timeout: 20 * time.Second},
		cache:        make(map[string]cacheEntry),
	}
}

func cleanCredential(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			value = strings.TrimSpace(value[1 : len(value)-1])
		}
	}
	return value
}

func (c *Client) Configured() bool {
	return c != nil && c.clientID != "" && c.clientSecret != ""
}

func (c *Client) token(ctx context.Context) (string, error) {
	if !c.Configured() {
		return "", fmt.Errorf("kroger API credentials are not configured")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.accessToken != "" && time.Now().Before(c.tokenExpiry.Add(-60*time.Second)) {
		return c.accessToken, nil
	}

	// product.compact is required for product prices; empty scope still authenticates
	// and is enough for location lookup if the Products scope isn't granted yet.
	scopes := []string{tokenScope, ""}
	var lastErr error
	for _, scope := range scopes {
		token, err := c.requestToken(ctx, scope, true)
		if err == nil {
			return token, nil
		}
		lastErr = err
		// Retry with client_id/secret in the body (some portals expect this).
		token, err = c.requestToken(ctx, scope, false)
		if err == nil {
			return token, nil
		}
		lastErr = err
	}
	return "", lastErr
}

func (c *Client) requestToken(ctx context.Context, scope string, useBasic bool) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	if scope != "" {
		form.Set("scope", scope)
	}
	if !useBasic {
		form.Set("client_id", c.clientID)
		form.Set("client_secret", c.clientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if useBasic {
		req.SetBasicAuth(c.clientID, c.clientSecret)
	}

	res, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	body, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		if res.StatusCode == http.StatusUnauthorized {
			return "", fmt.Errorf(
				"kroger rejected credentials (401). Confirm Client ID/Secret on the backend Vercel project (Production), that Products API is enabled, and redeploy. Details: %s",
				msg,
			)
		}
		return "", fmt.Errorf("kroger token error (%d): %s", res.StatusCode, msg)
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", err
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("kroger token response missing access_token")
	}

	expires := tr.ExpiresIn
	if expires <= 0 {
		expires = 1800
	}
	c.accessToken = tr.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(expires) * time.Second)
	return c.accessToken, nil
}

// Status is a safe diagnostic snapshot (never includes the secret).
func (c *Client) Status(ctx context.Context) map[string]any {
	out := map[string]any{
		"configured":      c.Configured(),
		"clientIdLength":  len(c.clientID),
		"clientIdPrefix":  prefix(c.clientID, 4),
		"secretConfigured": len(c.clientSecret) > 0,
		"secretLength":    len(c.clientSecret),
	}
	if !c.Configured() {
		out["tokenOk"] = false
		out["tokenError"] = "missing KROGER_CLIENT_ID or KROGER_CLIENT_SECRET"
		return out
	}
	_, err := c.token(ctx)
	if err != nil {
		out["tokenOk"] = false
		out["tokenError"] = err.Error()
		return out
	}
	out["tokenOk"] = true
	return out
}

func prefix(value string, n int) string {
	if value == "" {
		return ""
	}
	if len(value) <= n {
		return value
	}
	return value[:n] + "…"
}

func (c *Client) getJSON(ctx context.Context, endpoint string, query url.Values, dest any) error {
	token, err := c.token(ctx)
	if err != nil {
		return err
	}

	u := endpoint
	if len(query) > 0 {
		u = endpoint + "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	body, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return fmt.Errorf("kroger API error (%d): %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.Unmarshal(body, dest)
}

func (c *Client) SearchLocations(ctx context.Context, zip string, limit int) ([]Location, error) {
	zip = strings.TrimSpace(zip)
	if zip == "" {
		return nil, fmt.Errorf("zip code is required")
	}
	if limit <= 0 || limit > 20 {
		limit = 5
	}

	q := url.Values{}
	q.Set("filter.zipCode.near", zip)
	q.Set("filter.limit", fmt.Sprintf("%d", limit))

	var payload struct {
		Data []Location `json:"data"`
	}
	if err := c.getJSON(ctx, baseURL+"/locations", q, &payload); err != nil {
		return nil, err
	}
	return payload.Data, nil
}

func (c *Client) SearchProducts(ctx context.Context, term, locationID string, limit int) ([]Product, error) {
	term = strings.TrimSpace(term)
	locationID = strings.TrimSpace(locationID)
	if term == "" {
		return nil, fmt.Errorf("search term is required")
	}
	if locationID == "" {
		return nil, fmt.Errorf("locationId is required")
	}
	if limit <= 0 || limit > 50 {
		limit = 10
	}

	cacheKey := strings.ToLower(term) + "|" + locationID + "|" + fmt.Sprintf("%d", limit)
	c.cacheMu.Lock()
	if entry, ok := c.cache[cacheKey]; ok && time.Since(entry.at) < cacheTTL {
		products := entry.data
		c.cacheMu.Unlock()
		return products, nil
	}
	c.cacheMu.Unlock()

	q := url.Values{}
	q.Set("filter.term", term)
	q.Set("filter.locationId", locationID)
	q.Set("filter.limit", fmt.Sprintf("%d", limit))

	var payload struct {
		Data []Product `json:"data"`
	}
	if err := c.getJSON(ctx, baseURL+"/products", q, &payload); err != nil {
		return nil, err
	}

	c.cacheMu.Lock()
	c.cache[cacheKey] = cacheEntry{at: time.Now(), data: payload.Data}
	c.cacheMu.Unlock()

	return payload.Data, nil
}

// EffectivePrice returns promo if set and positive, otherwise regular.
func EffectivePrice(p Product) (float64, string, bool) {
	for _, item := range p.Items {
		if item.Price == nil {
			continue
		}
		price := item.Price.Regular
		if item.Price.Promo > 0 && (price <= 0 || item.Price.Promo < price) {
			price = item.Price.Promo
		}
		if price > 0 {
			size := strings.TrimSpace(item.Size)
			return price, size, true
		}
	}
	return 0, "", false
}
