package estimate

import "testing"

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
