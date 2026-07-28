package estimate

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

type ParsedLine struct {
	Quantity float64
	Unit     string
	Name     string
	Label    string
	Raw      string
}

var (
	qtyPattern  = `(?:\d+\s+\d+/\d+|\d+/\d+|\d+\.\d+|\d+)`
	unitAliases = map[string]string{
		"tsp": "tsp", "tsps": "tsp", "teaspoon": "tsp", "teaspoons": "tsp",
		"tbsp": "tbsp", "tbsps": "tbsp", "tablespoon": "tbsp", "tablespoons": "tbsp",
		"cup": "cup", "cups": "cup",
		"oz": "oz", "ounce": "oz", "ounces": "oz",
		"lb": "lb", "lbs": "lb", "pound": "lb", "pounds": "lb",
		"g": "g", "gram": "g", "grams": "g",
		"kg": "kg",
		"ml": "ml", "l": "l", "liter": "l", "litre": "l", "liters": "l", "litres": "l",
		"clove": "clove", "cloves": "clove",
		"pinch": "pinch", "pinches": "pinch",
		"can": "can", "cans": "can",
		"jar": "jar", "jars": "jar",
		"package": "package", "packages": "package", "pkg": "package",
		"slice": "slice", "slices": "slice",
		"bunch": "bunch", "bunches": "bunch",
		"ct": "count", "count": "count", "each": "count",
	}
	unitPattern = buildUnitPattern()
	// "32oz sugar", "2 cups flour", "3 1/2 cups sugar"
	withUnitRE = regexp.MustCompile(`(?i)^(` + qtyPattern + `)\s*(` + unitPattern + `)\b\.?(?:\s+of)?\s+(.+)$`)
	// "2 32oz chocolate chips" → 2 × 32 oz (size qty is not a mixed number)
	sizeQtyPattern   = `(?:\d+/\d+|\d+\.\d+|\d+)`
	countTimesSizeRE = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s+(` + sizeQtyPattern + `)\s*(` + unitPattern + `)\b\.?(?:\s+of)?\s+(.+)$`)
	// "1 (15-ounce) can black beans" / "2 (4 oz) cans diced green chile"
	sizedContainerRE = regexp.MustCompile(`(?i)^(` + qtyPattern + `)\s*\((\d+(?:\.\d+)?)\s*-?\s*(oz|ounce|ounces|g|gram|grams|lb|lbs|pound|pounds)\)\s+(cans?|jars?|packages?|pkgs?|boxes?|bags?)\s+(.+)$`)
	// "1 can black beans"
	plainContainerRE = regexp.MustCompile(`(?i)^(` + qtyPattern + `)\s+(cans?|jars?|packages?|pkgs?|boxes?|bags?)\s+(.+)$`)
	// "1 batch red enchilada sauce"
	batchRE   = regexp.MustCompile(`(?i)^(` + qtyPattern + `)\s+batch(?:es)?\s+(.+)$`)
	qtyOnlyRE = regexp.MustCompile(`(?i)^(` + qtyPattern + `)\s+(.+)$`)
	emojiRE   = regexp.MustCompile(`^[\x{1F300}-\x{1FAFF}\x{2600}-\x{27BF}\x{FE0F}\x{200D}]+\s*`)
	parensRE  = regexp.MustCompile(`\([^)]*\)`)
	prepCutRE = regexp.MustCompile(`(?i)\s+(?:diced|chopped|minced|sliced|cubed|cut)\s+into\b.*$`)
	trailPrepRE = regexp.MustCompile(`(?i),?\s*(?:rinsed and drained|drained and rinsed|rinsed|drained|patted dry|to taste|divided|softened|melted|room temperature)\s*$`)
)

func buildUnitPattern() string {
	keys := make([]string, 0, len(unitAliases))
	for k := range unitAliases {
		keys = append(keys, regexp.QuoteMeta(k))
	}
	// longer first
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if len(keys[j]) > len(keys[i]) {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return strings.Join(keys, "|")
}

func parseQuantity(raw string) float64 {
	trimmed := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	if parts := strings.Split(trimmed, " "); len(parts) == 2 && strings.Contains(parts[1], "/") {
		whole, _ := strconv.ParseFloat(parts[0], 64)
		frac := strings.Split(parts[1], "/")
		if len(frac) == 2 {
			n, _ := strconv.ParseFloat(frac[0], 64)
			d, _ := strconv.ParseFloat(frac[1], 64)
			if d != 0 {
				return whole + n/d
			}
		}
	}
	if strings.Contains(trimmed, "/") {
		frac := strings.Split(trimmed, "/")
		if len(frac) == 2 {
			n, _ := strconv.ParseFloat(frac[0], 64)
			d, _ := strconv.ParseFloat(frac[1], 64)
			if d != 0 {
				return n / d
			}
		}
	}
	v, _ := strconv.ParseFloat(trimmed, 64)
	return v
}

func normalizeName(value string) string {
	var b strings.Builder
	prevSpace := false
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '%' || r == '-' {
			b.WriteRune(r)
			prevSpace = false
			continue
		}
		if unicode.IsSpace(r) {
			if !prevSpace && b.Len() > 0 {
				b.WriteByte(' ')
				prevSpace = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func normalizeUnit(value string) string {
	key := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
	if alias, ok := unitAliases[key]; ok {
		return alias
	}
	return key
}

func ParseLine(line string) ParsedLine {
	cleaned := strings.TrimSpace(emojiRE.ReplaceAllString(strings.TrimSpace(line), ""))
	if cleaned == "" {
		return ParsedLine{Quantity: 1, Raw: line}
	}

	// "1 (15-ounce) can black beans (rinsed and drained)"
	if m := sizedContainerRE.FindStringSubmatch(cleaned); m != nil {
		label := strings.Join(strings.Fields(m[5]), " ")
		name := simplifyIngredientName(label)
		return ParsedLine{
			Quantity: parseQuantity(m[1]),
			Unit:     normalizeUnit(m[4]),
			Name:     name,
			Label:    label,
			Raw:      line,
		}
	}

	// "1 can black beans"
	if m := plainContainerRE.FindStringSubmatch(cleaned); m != nil {
		label := strings.Join(strings.Fields(m[3]), " ")
		return ParsedLine{
			Quantity: parseQuantity(m[1]),
			Unit:     normalizeUnit(m[2]),
			Name:     simplifyIngredientName(label),
			Label:    label,
			Raw:      line,
		}
	}

	// "1 batch red enchilada sauce"
	if m := batchRE.FindStringSubmatch(cleaned); m != nil {
		label := strings.Join(strings.Fields(m[2]), " ")
		return ParsedLine{
			Quantity: parseQuantity(m[1]),
			Unit:     "",
			Name:     simplifyIngredientName(label),
			Label:    label,
			Raw:      line,
		}
	}

	// Prefer mixed measures like "3 1/2 cups" over "count × size".
	if m := withUnitRE.FindStringSubmatch(cleaned); m != nil {
		label := strings.Join(strings.Fields(m[3]), " ")
		return ParsedLine{
			Quantity: parseQuantity(m[1]),
			Unit:     normalizeUnit(m[2]),
			Name:     simplifyIngredientName(label),
			Label:    label,
			Raw:      line,
		}
	}

	// "2 32oz chocolate chips" / "2 16 oz chocolate chips" → 64 oz / 32 oz
	if m := countTimesSizeRE.FindStringSubmatch(cleaned); m != nil {
		label := strings.Join(strings.Fields(m[4]), " ")
		count := parseQuantity(m[1])
		size := parseQuantity(m[2])
		return ParsedLine{
			Quantity: count * size,
			Unit:     normalizeUnit(m[3]),
			Name:     simplifyIngredientName(label),
			Label:    label,
			Raw:      line,
		}
	}

	if m := qtyOnlyRE.FindStringSubmatch(cleaned); m != nil {
		label := strings.Join(strings.Fields(m[2]), " ")
		return ParsedLine{
			Quantity: parseQuantity(m[1]),
			Unit:     "",
			Name:     simplifyIngredientName(label),
			Label:    label,
			Raw:      line,
		}
	}

	label := strings.Join(strings.Fields(cleaned), " ")
	return ParsedLine{
		Quantity: 1,
		Unit:     "",
		Name:     simplifyIngredientName(label),
		Label:    label,
		Raw:      line,
	}
}

// simplifyIngredientName strips prep notes and alternatives for grocery search.
func simplifyIngredientName(label string) string {
	label = parensRE.ReplaceAllString(label, " ")
	label = strings.Join(strings.Fields(label), " ")
	lower := strings.ToLower(label)
	if i := strings.Index(lower, " or "); i > 0 {
		label = strings.TrimSpace(label[:i])
		lower = strings.ToLower(label)
	}
	if m := prepCutRE.FindStringIndex(label); m != nil && m[0] > 0 {
		label = strings.TrimSpace(label[:m[0]])
	}
	label = trailPrepRE.ReplaceAllString(label, "")
	return normalizeName(label)
}

// SearchTerm strips parenthetical notes and applies ingredient search aliases.
func SearchTerm(name string) string {
	name = simplifyIngredientName(name)
	name = resolveSearchTerm(name)
	return ClampSearchTerm(name, 8)
}

// ClampSearchTerm keeps Kroger happy (PRODUCT-2019: max 8 terms).
func ClampSearchTerm(term string, maxWords int) string {
	term = strings.Join(strings.Fields(strings.TrimSpace(term)), " ")
	if maxWords <= 0 {
		maxWords = 8
	}
	fields := strings.Fields(term)
	if len(fields) > maxWords {
		fields = fields[:maxWords]
	}
	return strings.Join(fields, " ")
}
