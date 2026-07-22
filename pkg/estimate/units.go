package estimate

import (
	"regexp"
	"strconv"
	"strings"
)

// Density grams per cup for common kitchen ingredients (approximate).
var densityGPerCup = map[string]float64{
	"sugar":             200,
	"granulated sugar":  200,
	"white sugar":       200,
	"brown sugar":       220,
	"powdered sugar":    120,
	"confectioners sugar": 120,
	"flour":             120,
	"all purpose flour": 120,
	"all-purpose flour": 120,
	"bread flour":       127,
	"butter":            227,
	"milk":              245,
	"water":             237,
	"oil":               218,
	"olive oil":         216,
	"vegetable oil":     218,
	"honey":             340,
	"salt":              273,
	"rice":              185,
	"oats":              90,
	"cocoa":             85,
	"cocoa powder":      85,
	"yogurt":            245,
	"cream":             238,
	"sour cream":        230,
	"cheese":            113,
	"shredded cheese":   113,
	"cornmeal":          138,
	"cornstarch":        128,
	"baking powder":     220,
	"baking soda":       220,
}

var packageSizeRE = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(fl\s*oz|fluid\s*ounces?|oz|ounces?|lb|lbs|pounds?|g|grams?|kg|kilograms?|ml|l|liters?|litres?|ct|count|ea|each)\b`)

func densityFor(name string) (float64, bool) {
	n := strings.ToLower(strings.TrimSpace(name))
	if v, ok := densityGPerCup[n]; ok {
		return v, true
	}
	// substring / keyword match — prefer longer keys
	bestKey := ""
	bestVal := 0.0
	for key, val := range densityGPerCup {
		if strings.Contains(n, key) && len(key) > len(bestKey) {
			bestKey = key
			bestVal = val
		}
	}
	if bestKey != "" {
		return bestVal, true
	}
	return 0, false
}

// NeedToGrams converts a recipe quantity into grams when possible.
// For countable items with empty unit, returns count with ok=false and uses CountNeeded instead.
func NeedToGrams(qty float64, unit, name string) (grams float64, ok bool, reason string) {
	if qty <= 0 {
		return 0, false, "quantity must be positive"
	}
	unit = strings.ToLower(strings.TrimSpace(unit))

	switch unit {
	case "g":
		return qty, true, ""
	case "kg":
		return qty * 1000, true, ""
	case "oz":
		return qty * 28.3495, true, ""
	case "lb":
		return qty * 453.592, true, ""
	case "ml":
		// assume water-like unless density known; prefer density path for named ingredients
		if d, found := densityFor(name); found {
			return qty * (d / 237.0), true, ""
		}
		return qty, true, ""
	case "l":
		if d, found := densityFor(name); found {
			return qty * 1000 * (d / 237.0), true, ""
		}
		return qty * 1000, true, ""
	case "cup":
		d, found := densityFor(name)
		if !found {
			return 0, false, "no density for volume conversion"
		}
		return qty * d, true, ""
	case "tbsp":
		d, found := densityFor(name)
		if !found {
			return 0, false, "no density for volume conversion"
		}
		return qty * (d / 16.0), true, ""
	case "tsp":
		d, found := densityFor(name)
		if !found {
			return 0, false, "no density for volume conversion"
		}
		return qty * (d / 48.0), true, ""
	case "pinch":
		return qty * 0.3, true, ""
	case "", "count", "clove", "slice", "bunch", "can", "package":
		return 0, false, "count-based item"
	default:
		return 0, false, "unsupported unit"
	}
}

func IsCountBased(unit string) bool {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "", "count", "clove", "slice", "bunch", "can", "package":
		return true
	default:
		return false
	}
}

// PackageToGrams parses Kroger size strings like "4 lb", "16 oz", "1 gallon" into grams.
// Returns count for "12 ct" style with isCount=true.
func PackageToGrams(size string) (grams float64, count float64, isCount bool, ok bool) {
	size = strings.TrimSpace(size)
	if size == "" {
		return 0, 0, false, false
	}

	m := packageSizeRE.FindStringSubmatch(size)
	if m == nil {
		return 0, 0, false, false
	}
	amount, err := strconv.ParseFloat(m[1], 64)
	if err != nil || amount <= 0 {
		return 0, 0, false, false
	}
	unit := strings.ToLower(strings.ReplaceAll(m[2], " ", ""))

	switch {
	case unit == "g" || strings.HasPrefix(unit, "gram"):
		return amount, 0, false, true
	case unit == "kg" || strings.HasPrefix(unit, "kilogram"):
		return amount * 1000, 0, false, true
	case unit == "oz" || strings.HasPrefix(unit, "ounce"):
		return amount * 28.3495, 0, false, true
	case unit == "floz" || strings.HasPrefix(unit, "fluid"):
		return amount * 29.5735, 0, false, true // approx water density
	case unit == "lb" || unit == "lbs" || strings.HasPrefix(unit, "pound"):
		return amount * 453.592, 0, false, true
	case unit == "ml":
		return amount, 0, false, true
	case unit == "l" || strings.HasPrefix(unit, "liter") || strings.HasPrefix(unit, "litre"):
		return amount * 1000, 0, false, true
	case unit == "ct" || unit == "count" || unit == "ea" || unit == "each":
		return 0, amount, true, true
	default:
		return 0, 0, false, false
	}
}

func DollarPerGram(price, grams float64) float64 {
	if price <= 0 || grams <= 0 {
		return 0
	}
	return price / grams
}

func DollarPerCount(price, count float64) float64 {
	if price <= 0 || count <= 0 {
		return 0
	}
	return price / count
}
