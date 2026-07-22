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

func TestPackagesNeeded(t *testing.T) {
	// 32 oz / 11.5 oz → 3 bags
	need := 32 * 28.3495
	pkg := 11.5 * 28.3495
	got := packagesNeeded(need, pkg)
	if got != 3 {
		t.Fatalf("packagesNeeded=%v want 3", got)
	}
}
