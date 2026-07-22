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
		"package": "package", "packages": "package", "pkg": "package",
		"slice": "slice", "slices": "slice",
		"bunch": "bunch", "bunches": "bunch",
		"ct": "count", "count": "count", "each": "count",
	}
	unitPattern = buildUnitPattern()
	withUnitRE  = regexp.MustCompile(`(?i)^(` + qtyPattern + `)\s*(` + unitPattern + `)\b\.?(?:\s+of)?\s+(.+)$`)
	qtyOnlyRE   = regexp.MustCompile(`(?i)^(` + qtyPattern + `)\s+(.+)$`)
	emojiRE     = regexp.MustCompile(`^[\x{1F300}-\x{1FAFF}\x{2600}-\x{27BF}\x{FE0F}\x{200D}]+\s*`)
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

	if m := withUnitRE.FindStringSubmatch(cleaned); m != nil {
		label := strings.Join(strings.Fields(m[3]), " ")
		return ParsedLine{
			Quantity: parseQuantity(m[1]),
			Unit:     normalizeUnit(m[2]),
			Name:     normalizeName(label),
			Label:    label,
			Raw:      line,
		}
	}

	if m := qtyOnlyRE.FindStringSubmatch(cleaned); m != nil {
		label := strings.Join(strings.Fields(m[2]), " ")
		return ParsedLine{
			Quantity: parseQuantity(m[1]),
			Unit:     "",
			Name:     normalizeName(label),
			Label:    label,
			Raw:      line,
		}
	}

	label := strings.Join(strings.Fields(cleaned), " ")
	return ParsedLine{
		Quantity: 1,
		Unit:     "",
		Name:     normalizeName(label),
		Label:    label,
		Raw:      line,
	}
}

// SearchTerm strips parenthetical notes and applies ingredient search aliases.
func SearchTerm(name string) string {
	name = regexp.MustCompile(`\([^)]*\)`).ReplaceAllString(name, " ")
	name = normalizeName(name)
	return resolveSearchTerm(name)
}
