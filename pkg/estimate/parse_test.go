package estimate

import (
	"strings"
	"testing"
)

func TestParseGluedOz(t *testing.T) {
	p := ParseLine("32oz chocolate chips")
	if p.Quantity != 32 {
		t.Fatalf("quantity=%v want 32", p.Quantity)
	}
	if p.Unit != "oz" {
		t.Fatalf("unit=%q want oz", p.Unit)
	}
	if p.Name != "chocolate chips" {
		t.Fatalf("name=%q want chocolate chips", p.Name)
	}
}

func TestParseCountTimesSize(t *testing.T) {
	p := ParseLine("2 32oz chocolate chips")
	if p.Quantity != 64 {
		t.Fatalf("quantity=%v want 64", p.Quantity)
	}
	if p.Unit != "oz" {
		t.Fatalf("unit=%q want oz", p.Unit)
	}
	if p.Name != "chocolate chips" {
		t.Fatalf("name=%q want chocolate chips", p.Name)
	}

	p = ParseLine("2 16 oz chocolate chips")
	if p.Quantity != 32 || p.Unit != "oz" || p.Name != "chocolate chips" {
		t.Fatalf("got qty=%v unit=%q name=%q", p.Quantity, p.Unit, p.Name)
	}
}

func TestParseMixedCupsStillWorks(t *testing.T) {
	p := ParseLine("3 1/2 cups sugar")
	if p.Quantity != 3.5 {
		t.Fatalf("quantity=%v want 3.5", p.Quantity)
	}
	if p.Unit != "cup" {
		t.Fatalf("unit=%q want cup", p.Unit)
	}
	if p.Name != "sugar" {
		t.Fatalf("name=%q want sugar", p.Name)
	}
}

func TestParseSizedCan(t *testing.T) {
	p := ParseLine("1 (15-ounce) can black beans (rinsed and drained)")
	if p.Quantity != 1 || p.Unit != "can" || p.Name != "black beans" {
		t.Fatalf("got qty=%v unit=%q name=%q", p.Quantity, p.Unit, p.Name)
	}
	if term := SearchTerm(p.Name); term != "black beans" {
		t.Fatalf("search=%q", term)
	}
}

func TestParseOilAlternative(t *testing.T) {
	p := ParseLine("2 tbsp avocado oil (or olive oil)")
	if p.Unit != "tbsp" || p.Name != "avocado oil" {
		t.Fatalf("got unit=%q name=%q", p.Unit, p.Name)
	}
}

func TestParseBatchAndChicken(t *testing.T) {
	p := ParseLine("1 batch red enchilada sauce")
	if p.Name != "red enchilada sauce" {
		t.Fatalf("name=%q", p.Name)
	}
	p = ParseLine("1 1/2 lb boneless skinless chicken breasts (diced into small 1/2-inch pieces)")
	if p.Quantity != 1.5 || p.Unit != "lb" {
		t.Fatalf("qty=%v unit=%q", p.Quantity, p.Unit)
	}
	if p.Name != "boneless skinless chicken breasts" {
		t.Fatalf("name=%q", p.Name)
	}
	if term := SearchTerm(p.Name); len(strings.Fields(term)) > 8 {
		t.Fatalf("search too long: %q", term)
	}
}

func TestPackageCountFromDescription(t *testing.T) {
	count, ok := PackageCount("40 oz", "Mission Large Flour Burrito Tortillas 16 Count")
	if !ok || count != 16 {
		t.Fatalf("count=%v ok=%v", count, ok)
	}
}

func TestClampSearchTerm(t *testing.T) {
	got := ClampSearchTerm("a b c d e f g h i j", 8)
	if got != "a b c d e f g h" {
		t.Fatalf("got %q", got)
	}
}

func TestPackagesNeeded(t *testing.T) {
	// 32 oz / 11.5 oz → 3 bags
	need := 32 * 28.3495
	pkg := 11.5 * 28.3495
	got := packagesNeeded(need, pkg)
	if got != 3 {
		t.Fatalf("packagesNeeded=%v want 3", got)
	}
}

func TestLineCostPortionVsPackages(t *testing.T) {
	portion := lineCost(PricingPortion, 0.12, 3.99)
	if portion != 0.12 {
		t.Fatalf("portion=%v want 0.12", portion)
	}
	packs := lineCost(PricingPackages, 0.12, 3.99)
	if packs != 3.99 {
		t.Fatalf("packages=%v want 3.99", packs)
	}
}
