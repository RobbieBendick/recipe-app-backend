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
	rule := ruleFor(name)
	if rule == nil {
		rule = ruleFor(term)
	}

	if rule != nil {
		if containsAny(desc, rule.Exclude...) {
			return -1
		}
		score := 0
		preferred := false
		for _, pref := range rule.Prefer {
			pref = strings.ToLower(pref)
			if pref != "" && strings.Contains(desc, pref) {
				preferred = true
				score += 50
			}
		}
		// Require at least one search-term token in the description.
		tokens := strings.Fields(term)
		matched := 0
		for _, tok := range tokens {
			if len(tok) < 3 {
				continue
			}
			if strings.Contains(desc, tok) {
				matched++
				score += 15
			}
		}
		if matched == 0 && !preferred {
			return -1
		}
		if preferred {
			score += 40
		}
		return score
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
	if containsAny(desc, defaultPreparedFoodExclude...) {
		score -= 50
	}
	return score
}

func containsAny(haystack string, needles ...string) bool {
	for _, n := range needles {
		n = strings.ToLower(strings.TrimSpace(n))
		if n != "" && strings.Contains(haystack, n) {
			return true
		}
	}
	return false
}

func filterRelevantProducts(ingredientName, searchTerm string, products []kroger.Product) []kroger.Product {
	type scored struct {
		p     kroger.Product
		score int
	}
	ranked := make([]scored, 0, len(products))
	bestScore := -1
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

	// When we have strong preferred matches, keep only that tier.
	threshold := 0
	if bestScore >= 80 {
		threshold = 80
	} else if bestScore >= 40 {
		threshold = 40
	}

	out := make([]kroger.Product, 0, len(ranked))
	for _, item := range ranked {
		if item.score >= threshold {
			out = append(out, item.p)
		}
	}
	return out
}
