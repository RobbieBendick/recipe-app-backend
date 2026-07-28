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

// LineOverride forces a specific Kroger product (and optional search term) for a line.
type LineOverride struct {
	Input      string `json:"input"`
	ProductID  string `json:"productId"`
	SearchTerm string `json:"searchTerm,omitempty"`
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

// PricingMode controls how package prices become a line estimate.
type PricingMode string

const (
	// PricingPortion charges for the amount used (recipes).
	PricingPortion PricingMode = "portion"
	// PricingPackages rounds up to whole packages you must buy (shopping lists).
	PricingPackages PricingMode = "packages"
)

func NormalizePricingMode(value string) PricingMode {
	if strings.EqualFold(strings.TrimSpace(value), string(PricingPackages)) {
		return PricingPackages
	}
	return PricingPortion
}

func EstimateLines(ctx context.Context, client *kroger.Client, locationID string, lines []string, overrides []LineOverride, mode PricingMode, assist *Assist) (*Result, error) {
	mode = NormalizePricingMode(string(mode))
	byInput := map[string]LineOverride{}
	for _, o := range overrides {
		key := overrideKey(o.Input)
		if key == "" || strings.TrimSpace(o.ProductID) == "" {
			continue
		}
		byInput[key] = o
	}

	out := &Result{
		Currency:   "USD",
		LocationID: locationID,
		Lines:      make([]LineEstimate, 0, len(lines)),
	}

	type pending struct {
		raw      string
		parsed   ParsedLine
		override *LineOverride
		term     string
		prefer   []string
		exclude  []string
		products []kroger.Product
		le       LineEstimate
		done     bool
	}

	pendings := make([]*pending, 0, len(lines))
	names := make([]string, 0, len(lines))
	fallbacks := map[string]string{}

	for _, raw := range lines {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		var ov *LineOverride
		if o, ok := byInput[overrideKey(raw)]; ok {
			cp := o
			ov = &cp
		}
		parsed := ParseLine(raw)
		p := &pending{
			raw:      raw,
			parsed:   parsed,
			override: ov,
			le:       LineEstimate{Input: raw},
		}
		if parsed.Name == "" {
			p.le.Status = StatusSkipped
			p.le.Reason = "could not parse ingredient"
			p.done = true
		} else {
			fb := SearchTerm(parsed.Name)
			fallbacks[normalizeName(parsed.Name)] = fb
			names = append(names, parsed.Name)
			p.term = fb
		}
		pendings = append(pendings, p)
	}

	if assist.Enabled() {
		assist.WarmSearchHints(ctx, names, fallbacks)
	}

	choiceAsks := make([]productChoiceAsk, 0, len(pendings))
	for _, p := range pendings {
		if p.done {
			continue
		}
		term := p.term
		var prefer, exclude []string
		if assist.Enabled() {
			term, prefer, exclude = assist.resolveSearchTerm(p.parsed.Name, p.term)
		}
		if p.override != nil && strings.TrimSpace(p.override.SearchTerm) != "" {
			term = strings.TrimSpace(p.override.SearchTerm)
		}
		p.term = term
		p.prefer = prefer
		p.exclude = exclude
		p.le.SearchTerm = term

		if term == "" {
			p.le.Status = StatusSkipped
			p.le.Reason = "empty search term"
			p.done = true
			continue
		}
		if len(term) < 3 {
			p.le.Status = StatusSkipped
			p.le.Reason = "search term must be at least 3 characters"
			p.done = true
			continue
		}

		products, err := client.SearchProducts(ctx, term, locationID, 24)
		if err != nil {
			p.le.Status = StatusError
			p.le.Reason = err.Error()
			p.done = true
			continue
		}

		if p.override != nil {
			preferID := strings.TrimSpace(p.override.ProductID)
			if preferID != "" {
				if prod, ok := findProduct(products, preferID); ok {
					p.le = estimateWithProduct(p.parsed, prod, p.le, mode)
					p.done = true
					continue
				}
			}
		}

		filtered := filterRelevantProducts(p.parsed.Name, term, products, prefer, exclude)
		if len(filtered) == 0 {
			filtered = products
		}
		p.products = filtered
		if len(filtered) == 0 {
			p.le.Status = StatusSkipped
			p.le.Reason = "no matching products found"
			p.done = true
			continue
		}

		if assist.Enabled() && p.override == nil {
			cands := make([]productCandidate, 0, len(filtered))
			for _, prod := range filtered {
				price, size, ok := kroger.EffectivePrice(prod)
				if !ok {
					price, size = 0, ""
				}
				cands = append(cands, productCandidate{
					ProductID:   prod.ProductID,
					Description: prod.Description,
					Brand:       prod.Brand,
					Size:        size,
					Price:       price,
				})
			}
			choiceAsks = append(choiceAsks, productChoiceAsk{
				Key:        normalizeName(p.parsed.Name) + "|" + p.raw,
				Ingredient: p.raw,
				SearchTerm: term,
				Candidates: cands,
			})
		}
	}

	picks := map[string]string{}
	if assist.Enabled() && len(choiceAsks) > 0 {
		picks = assist.ChooseProducts(ctx, choiceAsks)
	}

	for _, p := range pendings {
		if p.done {
			out.Lines = append(out.Lines, p.le)
			if p.le.Status == StatusOK {
				out.Total += p.le.Estimate
			}
			continue
		}

		pickKey := normalizeName(p.parsed.Name) + "|" + p.raw
		if id := picks[pickKey]; id != "" {
			if prod, ok := findProduct(p.products, id); ok {
				line := estimateWithProduct(p.parsed, prod, p.le, mode)
				out.Lines = append(out.Lines, line)
				if line.Status == StatusOK {
					out.Total += line.Estimate
				}
				continue
			}
		}

		line := finalizeCheapest(p.parsed, p.products, p.le, mode)
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
func ListProductOptions(ctx context.Context, client *kroger.Client, locationID, raw, searchOverride string, mode PricingMode, assist *Assist) (*ProductOptionsResult, error) {
	mode = NormalizePricingMode(string(mode))
	raw = strings.TrimSpace(raw)
	parsed := ParseLine(raw)
	out := &ProductOptionsResult{
		Input:   raw,
		Options: []ProductOption{},
	}
	if parsed.Name == "" {
		return out, nil
	}
	fallback := SearchTerm(parsed.Name)
	term := strings.TrimSpace(searchOverride)
	var prefer, exclude []string
	if term == "" {
		if assist.Enabled() {
			assist.WarmSearchHints(ctx, []string{parsed.Name}, map[string]string{
				normalizeName(parsed.Name): fallback,
			})
			term, prefer, exclude = assist.resolveSearchTerm(parsed.Name, fallback)
		} else {
			term = fallback
		}
	}
	out.SearchTerm = term
	if term == "" || len(term) < 3 {
		return out, nil
	}

	products, err := client.SearchProducts(ctx, term, locationID, 24)
	if err != nil {
		return nil, err
	}
	relevant := filterRelevantProducts(parsed.Name, term, products, prefer, exclude)
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

	if assist.Enabled() && len(ordered) > 1 {
		cands := make([]productCandidate, 0, len(ordered))
		for _, prod := range ordered {
			price, size, ok := kroger.EffectivePrice(prod)
			if !ok {
				price, size = 0, ""
			}
			cands = append(cands, productCandidate{
				ProductID:   prod.ProductID,
				Description: prod.Description,
				Brand:       prod.Brand,
				Size:        size,
				Price:       price,
			})
		}
		picks := assist.ChooseProducts(ctx, []productChoiceAsk{{
			Key:        normalizeName(parsed.Name),
			Ingredient: raw,
			SearchTerm: term,
			Candidates: cands,
		}})
		if id := picks[normalizeName(parsed.Name)]; id != "" {
			reordered := make([]kroger.Product, 0, len(ordered))
			var chosen *kroger.Product
			rest := make([]kroger.Product, 0, len(ordered))
			for i := range ordered {
				if ordered[i].ProductID == id {
					cp := ordered[i]
					chosen = &cp
					continue
				}
				rest = append(rest, ordered[i])
			}
			if chosen != nil {
				reordered = append(reordered, *chosen)
				reordered = append(reordered, rest...)
				ordered = reordered
			}
		}
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
		opt, ok := productOption(parsed, p, countBased, gramsNeeded, mode)
		if !ok {
			continue
		}
		out.Options = append(out.Options, opt)
	}
	return out, nil
}

func finalizeCheapest(parsed ParsedLine, products []kroger.Product, le LineEstimate, mode PricingMode) LineEstimate {
	if len(products) == 0 {
		le.Status = StatusSkipped
		le.Reason = "no matching products found"
		return le
	}
	if IsCountBased(parsed.Unit) {
		return estimateCount(parsed, products, le, mode)
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
		cost := lineCost(mode, gramsNeeded*ppg, pkgs*price)
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

func estimateWithProduct(parsed ParsedLine, p kroger.Product, le LineEstimate, mode PricingMode) LineEstimate {
	if IsCountBased(parsed.Unit) {
		return estimateCount(parsed, []kroger.Product{p}, le, mode)
	}
	gramsNeeded, ok, reason := NeedToGrams(parsed.Quantity, parsed.Unit, parsed.Name)
	if !ok {
		le.Status = StatusSkipped
		le.Reason = reason
		return le
	}
	le.Grams = round3(gramsNeeded)
	opt, ok := productOption(parsed, p, false, gramsNeeded, mode)
	if !ok {
		le.Status = StatusSkipped
		le.Reason = "overridden product has no usable package size"
		return le
	}
	le.Status = StatusOK
	le.UnitPricePerGram = opt.UnitPricePerGram
	le.PackagesNeeded = opt.PackagesNeeded
	le.Estimate = opt.Estimate
	le.ProductID = opt.ProductID
	le.ProductDescription = opt.Description
	le.ProductSize = opt.Size
	le.ProductPrice = opt.Price
	return le
}

func findProduct(products []kroger.Product, productID string) (kroger.Product, bool) {
	for _, p := range products {
		if p.ProductID == productID {
			return p, true
		}
	}
	return kroger.Product{}, false
}

func overrideKey(input string) string {
	return strings.ToLower(strings.TrimSpace(input))
}

func estimateCount(parsed ParsedLine, products []kroger.Product, le LineEstimate, mode PricingMode) LineEstimate {
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
		per := DollarPerCount(price, pkgCount)
		if per <= 0 || pkgs <= 0 {
			continue
		}
		cost := lineCost(mode, need*per, pkgs*price)
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

func productOption(parsed ParsedLine, p kroger.Product, countBased bool, gramsNeeded float64, mode PricingMode) (ProductOption, bool) {
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
		opt.Estimate = roundMoney(lineCost(mode, need*per, pkgs*price))
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
	opt.Estimate = roundMoney(lineCost(mode, gramsNeeded*ppg, pkgs*price))
	return opt, true
}

func lineCost(mode PricingMode, portionCost, packageCost float64) float64 {
	if mode == PricingPackages {
		return packageCost
	}
	return portionCost
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
