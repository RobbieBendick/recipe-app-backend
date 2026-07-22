package estimate

import (
	"context"
	"math"
	"strings"

	"github.com/robbi/recipe-app-backend/pkg/kroger"
)

type LineStatus string

const (
	StatusOK      LineStatus = "ok"
	StatusSkipped LineStatus = "skipped"
	StatusError   LineStatus = "error"
)

type LineEstimate struct {
	Input              string     `json:"input"`
	Status             LineStatus `json:"status"`
	Reason             string     `json:"reason,omitempty"`
	SearchTerm         string     `json:"searchTerm,omitempty"`
	Grams              float64    `json:"grams,omitempty"`
	Count              float64    `json:"count,omitempty"`
	PackagesNeeded     float64    `json:"packagesNeeded,omitempty"`
	UnitPricePerGram   float64    `json:"unitPricePerGram,omitempty"`
	UnitPricePerCount  float64    `json:"unitPricePerCount,omitempty"`
	Estimate           float64    `json:"estimate,omitempty"`
	ProductID          string     `json:"productId,omitempty"`
	ProductDescription string     `json:"productDescription,omitempty"`
	ProductSize        string     `json:"productSize,omitempty"`
	ProductPrice       float64    `json:"productPrice,omitempty"`
}

// ProductOption is a Kroger hit the user can pick for an ingredient line.
type ProductOption struct {
	ProductID         string  `json:"productId"`
	Description       string  `json:"description"`
	Brand             string  `json:"brand,omitempty"`
	Size              string  `json:"size"`
	Price             float64 `json:"price"`
	Estimate          float64 `json:"estimate"`
	PackagesNeeded    float64 `json:"packagesNeeded,omitempty"`
	UnitPricePerGram  float64 `json:"unitPricePerGram,omitempty"`
	UnitPricePerCount float64 `json:"unitPricePerCount,omitempty"`
	Mode              string  `json:"mode"` // "weight" | "count"
}

type ProductOptionsResult struct {
	Input      string          `json:"input"`
	SearchTerm string          `json:"searchTerm"`
	Grams      float64         `json:"grams,omitempty"`
	Count      float64         `json:"count,omitempty"`
	Options    []ProductOption `json:"options"`
}

type Result struct {
	Currency   string         `json:"currency"`
	Total      float64        `json:"total"`
	Lines      []LineEstimate `json:"lines"`
	LocationID string         `json:"locationId"`
}

type StoreInfo struct {
	LocationID string `json:"locationId"`
	Name       string `json:"name"`
	Chain      string `json:"chain"`
	City       string `json:"city"`
	State      string `json:"state"`
	ZipCode    string `json:"zipCode"`
	Address    string `json:"address"`
}

func NearestStore(ctx context.Context, client *kroger.Client, zip string) (*StoreInfo, error) {
	locs, err := client.SearchLocations(ctx, zip, 5)
	if err != nil {
		return nil, err
	}
	if len(locs) == 0 {
		return nil, nil
	}
	loc := locs[0]
	return &StoreInfo{
		LocationID: kroger.NormalizeLocationID(loc.LocationID),
		Name:       loc.Name,
		Chain:      loc.Chain,
		City:       loc.Address.City,
		State:      loc.Address.State,
		ZipCode:    loc.Address.ZipCode,
		Address:    loc.Address.AddressLine1,
	}, nil
}

func EstimateLines(ctx context.Context, client *kroger.Client, locationID string, lines []string) (*Result, error) {
	out := &Result{
		Currency:   "USD",
		LocationID: locationID,
		Lines:      make([]LineEstimate, 0, len(lines)),
	}

	for _, raw := range lines {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		line := estimateOne(ctx, client, locationID, raw)
		out.Lines = append(out.Lines, line)
		if line.Status == StatusOK {
			out.Total += line.Estimate
		}
	}

	out.Total = roundMoney(out.Total)
	return out, nil
}

// ListProductOptions returns priced Kroger products for an ingredient line.
// searchOverride, when non-empty, replaces the derived SearchTerm for the Kroger query.
func ListProductOptions(ctx context.Context, client *kroger.Client, locationID, raw, searchOverride string) (*ProductOptionsResult, error) {
	raw = strings.TrimSpace(raw)
	parsed := ParseLine(raw)
	out := &ProductOptionsResult{
		Input:   raw,
		Options: []ProductOption{},
	}
	if parsed.Name == "" {
		return out, nil
	}
	term := strings.TrimSpace(searchOverride)
	if term == "" {
		term = SearchTerm(parsed.Name)
	}
	out.SearchTerm = term
	if term == "" || len(term) < 3 {
		return out, nil
	}

	products, err := client.SearchProducts(ctx, term, locationID, 24)
	if err != nil {
		return nil, err
	}
	// Keep relevance filter for ranking, but also surface other priced hits
	// so the user can pick something the auto-matcher skipped.
	relevant := filterRelevantProducts(parsed.Name, term, products)
	seen := map[string]bool{}
	ordered := make([]kroger.Product, 0, len(products))
	for _, p := range relevant {
		id := productKey(p)
		if seen[id] {
			continue
		}
		seen[id] = true
		ordered = append(ordered, p)
	}
	for _, p := range products {
		id := productKey(p)
		if seen[id] {
			continue
		}
		seen[id] = true
		ordered = append(ordered, p)
	}

	countBased := IsCountBased(parsed.Unit)
	var gramsNeeded float64
	if !countBased {
		g, ok, _ := NeedToGrams(parsed.Quantity, parsed.Unit, parsed.Name)
		if ok {
			gramsNeeded = g
			out.Grams = round3(g)
		}
	} else {
		need := parsed.Quantity
		if need <= 0 {
			need = 1
		}
		out.Count = need
	}

	for _, p := range ordered {
		opt, ok := productOption(parsed, p, countBased, gramsNeeded)
		if !ok {
			continue
		}
		out.Options = append(out.Options, opt)
	}
	return out, nil
}

func estimateOne(ctx context.Context, client *kroger.Client, locationID, raw string) LineEstimate {
	parsed := ParseLine(raw)
	le := LineEstimate{Input: raw}

	if parsed.Name == "" {
		le.Status = StatusSkipped
		le.Reason = "could not parse ingredient"
		return le
	}

	term := SearchTerm(parsed.Name)
	le.SearchTerm = term
	if term == "" {
		le.Status = StatusSkipped
		le.Reason = "empty search term"
		return le
	}
	if len(term) < 3 {
		le.Status = StatusSkipped
		le.Reason = "search term must be at least 3 characters"
		return le
	}

	products, err := client.SearchProducts(ctx, term, locationID, 12)
	if err != nil {
		le.Status = StatusError
		le.Reason = err.Error()
		return le
	}
	products = filterRelevantProducts(parsed.Name, term, products)
	if len(products) == 0 {
		le.Status = StatusSkipped
		le.Reason = "no matching products found"
		return le
	}

	if IsCountBased(parsed.Unit) {
		return estimateCount(parsed, products, le)
	}

	gramsNeeded, ok, reason := NeedToGrams(parsed.Quantity, parsed.Unit, parsed.Name)
	if !ok {
		le.Status = StatusSkipped
		le.Reason = reason
		return le
	}
	le.Grams = round3(gramsNeeded)

	bestCost := 0.0
	bestPricePerG := 0.0
	var best kroger.Product
	var bestSize string
	var bestPrice float64
	var bestPkgs float64
	found := false

	for _, p := range products {
		price, size, ok := kroger.EffectivePrice(p)
		if !ok {
			continue
		}
		pkgG, _, isCount, ok := PackageToGrams(size)
		if !ok || isCount || pkgG <= 0 {
			continue
		}
		ppg := DollarPerGram(price, pkgG)
		if ppg <= 0 {
			continue
		}
		pkgs := packagesNeeded(gramsNeeded, pkgG)
		cost := pkgs * price
		if !found || cost < bestCost || (cost == bestCost && ppg < bestPricePerG) {
			found = true
			bestCost = cost
			bestPricePerG = ppg
			best = p
			bestSize = size
			bestPrice = price
			bestPkgs = pkgs
		}
	}

	if !found {
		le.Status = StatusSkipped
		le.Reason = "no priced packages with a parseable weight"
		return le
	}

	le.Status = StatusOK
	le.UnitPricePerGram = round6(bestPricePerG)
	le.PackagesNeeded = bestPkgs
	le.Estimate = roundMoney(bestCost)
	le.ProductID = best.ProductID
	le.ProductDescription = best.Description
	le.ProductSize = bestSize
	le.ProductPrice = bestPrice
	return le
}

func estimateCount(parsed ParsedLine, products []kroger.Product, le LineEstimate) LineEstimate {
	need := parsed.Quantity
	if need <= 0 {
		need = 1
	}
	le.Count = need

	bestCost := 0.0
	bestPer := 0.0
	var best kroger.Product
	var bestSize string
	var bestPrice float64
	var bestPkgs float64
	found := false

	for _, p := range products {
		price, size, ok := kroger.EffectivePrice(p)
		if !ok {
			continue
		}
		_, count, isCount, ok := PackageToGrams(size)
		if !ok {
			continue
		}
		pkgCount := 1.0
		if isCount && count > 0 {
			pkgCount = count
		}
		pkgs := packagesNeeded(need, pkgCount)
		cost := pkgs * price
		per := DollarPerCount(price, pkgCount)
		if per <= 0 || pkgs <= 0 {
			continue
		}
		if !found || cost < bestCost || (cost == bestCost && per < bestPer) {
			found = true
			bestCost = cost
			bestPer = per
			best = p
			bestSize = size
			bestPrice = price
			bestPkgs = pkgs
		}
	}

	if !found {
		le.Status = StatusSkipped
		le.Reason = "no priced countable packages"
		return le
	}

	le.Status = StatusOK
	le.UnitPricePerCount = round6(bestPer)
	le.PackagesNeeded = bestPkgs
	le.Estimate = roundMoney(bestCost)
	le.ProductID = best.ProductID
	le.ProductDescription = best.Description
	le.ProductSize = bestSize
	le.ProductPrice = bestPrice
	return le
}

func productOption(parsed ParsedLine, p kroger.Product, countBased bool, gramsNeeded float64) (ProductOption, bool) {
	price, size, ok := kroger.EffectivePrice(p)
	if !ok {
		return ProductOption{}, false
	}
	opt := ProductOption{
		ProductID:   p.ProductID,
		Description: p.Description,
		Brand:       p.Brand,
		Size:        size,
		Price:       price,
	}

	if countBased {
		need := parsed.Quantity
		if need <= 0 {
			need = 1
		}
		_, count, isCount, ok := PackageToGrams(size)
		if !ok {
			return ProductOption{}, false
		}
		pkgCount := 1.0
		if isCount && count > 0 {
			pkgCount = count
		}
		pkgs := packagesNeeded(need, pkgCount)
		per := DollarPerCount(price, pkgCount)
		if per <= 0 || pkgs <= 0 {
			return ProductOption{}, false
		}
		opt.Mode = "count"
		opt.UnitPricePerCount = round6(per)
		opt.PackagesNeeded = pkgs
		opt.Estimate = roundMoney(pkgs * price)
		return opt, true
	}

	if gramsNeeded <= 0 {
		return ProductOption{}, false
	}
	pkgG, _, isCount, ok := PackageToGrams(size)
	if !ok || isCount || pkgG <= 0 {
		return ProductOption{}, false
	}
	ppg := DollarPerGram(price, pkgG)
	if ppg <= 0 {
		return ProductOption{}, false
	}
	pkgs := packagesNeeded(gramsNeeded, pkgG)
	opt.Mode = "weight"
	opt.UnitPricePerGram = round6(ppg)
	opt.PackagesNeeded = pkgs
	opt.Estimate = roundMoney(pkgs * price)
	return opt, true
}

func productKey(p kroger.Product) string {
	if p.ProductID != "" {
		return p.ProductID
	}
	return p.UPC + "|" + p.Description
}

// packagesNeeded is how many whole packages to buy to cover needAmount of the same unit.
func packagesNeeded(needAmount, packageAmount float64) float64 {
	if needAmount <= 0 || packageAmount <= 0 {
		return 0
	}
	return math.Ceil((needAmount / packageAmount) - 1e-9)
}

func roundMoney(v float64) float64 {
	return math.Round(v*100) / 100
}

func round3(v float64) float64 {
	return math.Round(v*1000) / 1000
}

func round6(v float64) float64 {
	return math.Round(v*1e6) / 1e6
}
