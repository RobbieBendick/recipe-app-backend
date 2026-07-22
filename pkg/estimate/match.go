package estimate

import (
	"strings"

	"github.com/robbi/recipe-app-backend/pkg/kroger"
)

// productScore ranks how well a Kroger product matches the ingredient.
// Higher is better. Negative means reject.
func productScore(ingredientName, searchTerm string, p kroger.Product) int {
	desc := strings.ToLower(strings.TrimSpace(p.Description + " " + p.Brand))
	if desc == "" {
		return -1
	}
	name := normalizeName(ingredientName)
	term := normalizeName(searchTerm)

	// Vanilla in recipes means extract, not ice cream / pudding / yogurt.
	if isVanillaExtractNeed(name, term) {
		if containsAny(desc, "ice cream", "frozen dairy", "yogurt", "pudding", "creamer", "coffee creamer", "candle", "air freshener", "protein") {
			return -1
		}
		if strings.Contains(desc, "extract") {
			return 100
		}
		if strings.Contains(desc, "vanilla") && !containsAny(desc, "flavor", "flavored") {
			return 40
		}
		if strings.Contains(desc, "vanilla") {
			return 10
		}
		return -1
	}

	score := 0
	tokens := strings.Fields(term)
	if len(tokens) == 0 {
		tokens = strings.Fields(name)
	}
	matched := 0
	for _, tok := range tokens {
		if len(tok) < 3 {
			continue
		}
		if strings.Contains(desc, tok) {
			matched++
			score += 20
		}
	}
	if matched == 0 {
		return -1
	}
	// Prefer simpler pantry items over prepared foods when scoring is close.
	if containsAny(desc, "ice cream", "frozen meal", "pizza", "cookie", "cake mix") {
		score -= 50
	}
	return score
}

func isVanillaExtractNeed(name, term string) bool {
	return name == "vanilla" ||
		name == "vanilla essence" ||
		strings.Contains(name, "vanilla extract") ||
		term == "vanilla extract"
}

func containsAny(haystack string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(haystack, n) {
			return true
		}
	}
	return false
}

func filterRelevantProducts(ingredientName, searchTerm string, products []kroger.Product) []kroger.Product {
	out := make([]kroger.Product, 0, len(products))
	bestScore := -1
	type scored struct {
		p     kroger.Product
		score int
	}
	ranked := make([]scored, 0, len(products))
	for _, p := range products {
		s := productScore(ingredientName, searchTerm, p)
		if s < 0 {
			continue
		}
		ranked = append(ranked, scored{p: p, score: s})
		if s > bestScore {
			bestScore = s
		}
	}
	// Keep only top-tier matches when we have strong hits (e.g. "extract").
	threshold := 0
	if bestScore >= 80 {
		threshold = 80
	} else if bestScore >= 40 {
		threshold = 40
	}
	for _, item := range ranked {
		if item.score >= threshold {
			out = append(out, item.p)
		}
	}
	return out
}
