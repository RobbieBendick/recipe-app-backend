package estimate

import "strings"

// IngredientRule remaps ambiguous recipe names to better Kroger searches
// and ranks/rejects product results. Add rows here as mismatches show up.
type IngredientRule struct {
	// Exact normalized ingredient names that trigger this rule.
	Names []string
	// Also match when the normalized name contains any of these phrases.
	NameContains []string
	// Kroger product search query (required for remapping).
	SearchAs string
	// Prefer products whose description/brand contains these (strong boost).
	Prefer []string
	// Reject products whose description/brand contains these.
	Exclude []string
}

// ingredientRules is checked in order; first match wins.
var ingredientRules = []IngredientRule{
	{
		Names:        []string{"vanilla", "vanilla extract", "pure vanilla extract", "vanilla essence"},
		NameContains: []string{"vanilla extract"},
		SearchAs:     "vanilla extract",
		Prefer:       []string{"extract"},
		Exclude: []string{
			"ice cream", "frozen dairy", "yogurt", "pudding", "creamer",
			"coffee creamer", "candle", "air freshener", "protein",
		},
	},
	{
		Names:    []string{"cinnamon", "ground cinnamon"},
		SearchAs: "ground cinnamon",
		Prefer:   []string{"ground cinnamon", "cinnamon"},
		Exclude: []string{
			"roll", "bun", "toast crunch", "cereal", "ice cream", "candy", "gum",
		},
	},
	{
		Names:    []string{"nutmeg", "ground nutmeg"},
		SearchAs: "ground nutmeg",
		Prefer:   []string{"nutmeg"},
		Exclude:  []string{"ice cream", "cookie", "candle"},
	},
	{
		Names:    []string{"ginger", "ground ginger"},
		SearchAs: "ground ginger",
		Prefer:   []string{"ground ginger", "ginger"},
		Exclude: []string{
			"ale", "beer", "soda", "candy", "cookie", "snap", "tea", "ice cream",
		},
	},
	{
		Names:    []string{"cream", "heavy cream", "whipping cream", "heavy whipping cream"},
		SearchAs: "heavy whipping cream",
		Prefer:   []string{"heavy cream", "whipping cream", "heavy whipping"},
		Exclude: []string{
			"ice cream", "creamer", "coffee creamer", "sour cream", "cream cheese",
			"soup", "cracker",
		},
	},
	{
		Names:    []string{"butter"},
		SearchAs: "butter",
		Prefer:   []string{"butter"},
		Exclude: []string{
			"cookie", "candy", "popcorn", "syrup", "spray", "substitute",
			"almond butter", "peanut butter", "cashew butter",
		},
	},
	{
		Names:    []string{"flour", "all purpose flour", "all-purpose flour", "ap flour"},
		SearchAs: "all purpose flour",
		Prefer:   []string{"flour"},
		Exclude: []string{
			"tortilla", "wrap", "mix", "bread", "muffin", "cake", "pasta", "noodle",
		},
	},
	{
		Names:    []string{"sugar", "white sugar", "granulated sugar"},
		SearchAs: "granulated sugar",
		Prefer:   []string{"granulated", "sugar"},
		Exclude: []string{
			"candy", "cereal", "cookie", "ice cream", "syrup", "substitute",
		},
	},
	{
		Names:    []string{"cocoa", "cocoa powder", "unsweetened cocoa"},
		SearchAs: "cocoa powder",
		Prefer:   []string{"cocoa powder", "cocoa"},
		Exclude: []string{
			"mix", "hot cocoa", "cereal", "candy", "ice cream", "pudding",
		},
	},
	{
		Names:    []string{"oil", "vegetable oil", "cooking oil"},
		SearchAs: "vegetable oil",
		Prefer:   []string{"vegetable oil", "canola oil", "cooking oil"},
		Exclude: []string{
			"olive", "spray", "essential", "hair", "skin", "supplement",
		},
	},
}

// Global soft penalties applied when no Prefer rule boosts the product.
var defaultPreparedFoodExclude = []string{
	"ice cream", "frozen meal", "pizza", "cookie", "cake mix", "candy",
}

func ruleFor(name string) *IngredientRule {
	n := normalizeName(name)
	if n == "" {
		return nil
	}
	for i := range ingredientRules {
		r := &ingredientRules[i]
		for _, exact := range r.Names {
			if n == normalizeName(exact) {
				return r
			}
		}
		for _, frag := range r.NameContains {
			if frag != "" && strings.Contains(n, normalizeName(frag)) {
				return r
			}
		}
	}
	return nil
}

// resolveSearchTerm returns the Kroger query for an ingredient name.
func resolveSearchTerm(name string) string {
	if r := ruleFor(name); r != nil && r.SearchAs != "" {
		return r.SearchAs
	}
	return name
}
