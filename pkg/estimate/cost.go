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
	Grams              float64    `json:"grams,omitempty"`
	Count              float64    `json:"count,omitempty"`
	UnitPricePerGram   float64    `json:"unitPricePerGram,omitempty"`
	UnitPricePerCount  float64    `json:"unitPricePerCount,omitempty"`
	Estimate           float64    `json:"estimate,omitempty"`
	ProductDescription string     `json:"productDescription,omitempty"`
	ProductSize        string     `json:"productSize,omitempty"`
	ProductPrice       float64    `json:"productPrice,omitempty"`
}

type Result struct {
	Currency  string         `json:"currency"`
	Total     float64        `json:"total"`
	Lines     []LineEstimate `json:"lines"`
	LocationID string        `json:"locationId"`
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

func estimateOne(ctx context.Context, client *kroger.Client, locationID, raw string) LineEstimate {
	parsed := ParseLine(raw)
	le := LineEstimate{Input: raw}

	if parsed.Name == "" {
		le.Status = StatusSkipped
		le.Reason = "could not parse ingredient"
		return le
	}

	term := SearchTerm(parsed.Name)
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

	bestPricePerG := 0.0
	var best kroger.Product
	var bestSize string
	var bestPrice float64
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
		if !found || ppg < bestPricePerG {
			found = true
			bestPricePerG = ppg
			best = p
			bestSize = size
			bestPrice = price
		}
	}

	if !found {
		le.Status = StatusSkipped
		le.Reason = "no priced packages with a parseable weight"
		return le
	}

	le.Status = StatusOK
	le.UnitPricePerGram = round6(bestPricePerG)
	le.Estimate = roundMoney(gramsNeeded * bestPricePerG)
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

	bestPer := 0.0
	var best kroger.Product
	var bestSize string
	var bestPrice float64
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
		per := 0.0
		if isCount && count > 0 {
			per = DollarPerCount(price, count)
		} else {
			// treat whole package as 1 unit when size isn't countable (e.g. "bunch")
			per = price
			count = 1
		}
		if per <= 0 {
			continue
		}
		if !found || per < bestPer {
			found = true
			bestPer = per
			best = p
			bestSize = size
			bestPrice = price
		}
	}

	if !found {
		le.Status = StatusSkipped
		le.Reason = "no priced countable packages"
		return le
	}

	le.Status = StatusOK
	le.UnitPricePerCount = round6(bestPer)
	le.Estimate = roundMoney(need * bestPer)
	le.ProductDescription = best.Description
	le.ProductSize = bestSize
	le.ProductPrice = bestPrice
	return le
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
