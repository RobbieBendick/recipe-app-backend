package estimate

import (
	"regexp"
	"strconv"
	"strings"
)

// Density grams per US cup for common shopping / pantry items (approximate).
// Sources: King Arthur Baking weight chart, USDA FoodData Central averages,
// and common culinary conversion tables. Values are spoon-and-level style
// where that matters (flours, powders).
var densityGPerCup = map[string]float64{
	// Liquids
	"water":                   237,
	"milk":                    245,
	"whole milk":              245,
	"2% milk":                 244,
	"skim milk":               245,
	"buttermilk":              245,
	"cream":                   238,
	"heavy cream":             238,
	"heavy whipping cream":    238,
	"whipping cream":          238,
	"half and half":           242,
	"half-and-half":           242,
	"evaporated milk":         252,
	"sweetened condensed milk": 306,
	"coconut milk":            240,
	"almond milk":             240,
	"oat milk":                240,
	"soy milk":                243,
	"broth":                   240,
	"stock":                   240,
	"chicken broth":           240,
	"beef broth":              240,
	"vegetable broth":         240,
	"coffee":                  237,
	"espresso":                237,
	"orange juice":            248,
	"lemon juice":             244,
	"lime juice":              244,
	"apple cider":             248,
	"apple juice":             248,
	"vinegar":                 239,
	"apple cider vinegar":     239,
	"white vinegar":           239,
	"balsamic vinegar":        255,
	"soy sauce":               255,
	"worcestershire sauce":    275,
	"hot sauce":               240,
	"fish sauce":              280,
	"vanilla extract":         208,
	"pure vanilla extract":    208,
	"vanilla":                 208,
	"almond extract":          208,
	"lemon extract":           208,

	// Fats & oils
	"butter":           227,
	"unsalted butter":  227,
	"salted butter":    227,
	"melted butter":    227,
	"margarine":        227,
	"ghee":             218,
	"shortening":       205,
	"lard":             205,
	"oil":              218,
	"vegetable oil":    218,
	"canola oil":       218,
	"cooking oil":      218,
	"olive oil":        216,
	"extra virgin olive oil": 216,
	"coconut oil":      218,
	"avocado oil":      218,
	"sesame oil":       218,
	"peanut oil":       218,
	"sunflower oil":    218,
	"mayonnaise":       220,
	"mayo":             220,
	"peanut butter":    258,
	"almond butter":    250,
	"tahini":           240,

	// Dairy solids
	"yogurt":              245,
	"greek yogurt":        245,
	"sour cream":          230,
	"creme fraiche":       232,
	"cream cheese":        232,
	"ricotta":             246,
	"ricotta cheese":      246,
	"cottage cheese":      226,
	"mascarpone":          226,
	"cheese":              113,
	"shredded cheese":     113,
	"grated cheese":       100,
	"cheddar":             113,
	"cheddar cheese":      113,
	"mozzarella":          112,
	"parmesan":            100,
	"parmesan cheese":     100,
	"grated parmesan":     100,
	"feta":                150,
	"feta cheese":         150,

	// Sugars & sweeteners
	"sugar":               200,
	"granulated sugar":    200,
	"white sugar":         200,
	"cane sugar":          200,
	"brown sugar":         220,
	"light brown sugar":   220,
	"dark brown sugar":    220,
	"powdered sugar":      120,
	"confectioners sugar": 120,
	"icing sugar":         120,
	"coconut sugar":       180,
	"turbinado sugar":     200,
	"raw sugar":           200,
	"honey":               340,
	"maple syrup":         322,
	"molasses":            328,
	"corn syrup":          340,
	"agave":               336,
	"agave nectar":        336,
	"golden syrup":        328,

	// Flours & starches
	"flour":               120,
	"all purpose flour":   120,
	"all-purpose flour":   120,
	"ap flour":            120,
	"bread flour":         127,
	"cake flour":          114,
	"pastry flour":        113,
	"self rising flour":   113,
	"self-rising flour":   113,
	"whole wheat flour":   120,
	"wheat flour":         120,
	"almond flour":        96,
	"oat flour":           88,
	"coconut flour":       112,
	"rice flour":          158,
	"cornmeal":            138,
	"corn meal":           138,
	"polenta":             138,
	"cornstarch":          128,
	"corn starch":         128,
	"potato starch":       152,
	"tapioca starch":      120,
	"tapioca flour":       120,
	"arrowroot":           128,
	"breadcrumbs":         108,
	"bread crumbs":        108,
	"panko":               50,

	// Leaveners & seasonings
	"baking powder":       192,
	"baking soda":         220,
	"salt":                273,
	"table salt":          292,
	"kosher salt":         240,
	"sea salt":            273,
	"pepper":              116,
	"black pepper":        116,
	"cinnamon":            125,
	"ground cinnamon":     125,
	"nutmeg":              112,
	"ground nutmeg":       112,
	"ginger":              88,
	"ground ginger":       88,
	"garlic powder":       128,
	"onion powder":        112,
	"paprika":             108,
	"chili powder":        120,
	"cumin":               96,
	"ground cumin":        96,
	"oregano":             72,
	"dried oregano":       72,
	"basil":               60,
	"dried basil":         60,
	"thyme":               72,
	"dried thyme":         72,
	"rosemary":            56,
	"dried rosemary":      56,
	"parsley":             48,
	"dried parsley":       48,
	"red pepper flakes":   72,
	"cayenne":             88,
	"turmeric":            112,
	"curry powder":        104,
	"italian seasoning":   72,
	"yeast":               150,
	"active dry yeast":    150,
	"instant yeast":       150,

	// Cocoa & chocolate
	"cocoa":               85,
	"cocoa powder":        85,
	"unsweetened cocoa":   85,
	"dutch cocoa":         85,
	"chocolate chips":     170,
	"mini chocolate chips": 180,
	"chocolate":           170,
	"semi sweet chocolate chips": 170,
	"dark chocolate chips": 170,
	"white chocolate chips": 170,
	"cacao nibs":          120,

	// Grains & legumes (dry / uncooked)
	"rice":                185,
	"white rice":          185,
	"long grain rice":     185,
	"brown rice":          190,
	"jasmine rice":        185,
	"basmati rice":        185,
	"arborio rice":        200,
	"oats":                90,
	"rolled oats":         90,
	"old fashioned oats":  90,
	"quick oats":          80,
	"steel cut oats":      176,
	"quinoa":              170,
	"couscous":            173,
	"barley":              200,
	"farro":               190,
	"bulgur":              140,
	"pasta":               100,
	"dry pasta":           100,
	"lentils":             192,
	"dry lentils":         192,
	"beans":               184,
	"dry beans":           184,
	"black beans":         184,
	"chickpeas":           200,
	"garbanzo beans":      200,
	"split peas":          200,

	// Nuts & seeds
	"almonds":             143,
	"sliced almonds":      92,
	"slivered almonds":    108,
	"walnuts":             120,
	"chopped walnuts":     120,
	"pecans":              110,
	"chopped pecans":      110,
	"cashews":             130,
	"peanuts":             146,
	"pistachios":          123,
	"hazelnuts":           135,
	"macadamia nuts":      134,
	"pine nuts":           135,
	"sesame seeds":        144,
	"chia seeds":          160,
	"flax seeds":          168,
	"flaxseed":            168,
	"sunflower seeds":     140,
	"pumpkin seeds":       140,
	"hemp seeds":          160,

	// Produce (chopped / prepared volume — rough)
	"onion":               160,
	"onions":              160,
	"chopped onion":       160,
	"diced onion":         160,
	"garlic":              136,
	"minced garlic":       136,
	"tomato":              180,
	"tomatoes":            180,
	"diced tomatoes":      180,
	"cherry tomatoes":     150,
	"potato":              150,
	"potatoes":            150,
	"carrot":              128,
	"carrots":             128,
	"celery":              120,
	"bell pepper":         150,
	"spinach":             30,
	"lettuce":             55,
	"cabbage":             90,
	"broccoli":            90,
	"cauliflower":         100,
	"mushroom":            70,
	"mushrooms":           70,
	"zucchini":            125,
	"cucumber":            120,
	"avocado":             150,
	"banana":              150,
	"apple":               125,
	"apples":              125,
	"berries":             140,
	"strawberries":        150,
	"blueberries":         150,
	"raspberries":         125,
	"lemon zest":          96,
	"lime zest":           96,
	"orange zest":         96,

	// Dried fruit & misc pantry
	"raisins":             165,
	"dried cranberries":   120,
	"dates":               150,
	"dried apricots":      130,
	"shredded coconut":    85,
	"coconut flakes":      85,
	"desiccated coconut":  85,
	"jam":                 320,
	"jelly":               320,
	"applesauce":          244,
	"pumpkin puree":       245,
	"tomato paste":        262,
	"tomato sauce":        245,
	"salsa":               240,
	"ketchup":             240,
	"mustard":             250,
	"dijon mustard":       250,
	"relish":              245,
	"pickles":             160,
	"olives":              140,
	"capers":              140,
	"bread":               120,
	"panko breadcrumbs":   50,
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
	case "", "count", "clove", "slice", "bunch", "can", "jar", "package":
		return true
	default:
		return false
	}
}

var countInTextRE = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(?:ct|count|pk|pack)\b`)

// PackageCount returns how many discrete items are in a Kroger package.
// Prefers explicit count sizing ("16 ct") and falls back to counts in the description
// when size is weight-only (e.g. tortillas: size "40 oz", description "16 Count").
func PackageCount(size, description string) (float64, bool) {
	if _, count, isCount, ok := PackageToGrams(size); ok && isCount && count > 0 {
		return count, true
	}
	for _, text := range []string{description, size} {
		if m := countInTextRE.FindStringSubmatch(text); m != nil {
			amount, err := strconv.ParseFloat(m[1], 64)
			if err == nil && amount > 0 {
				return amount, true
			}
		}
	}
	return 0, false
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
