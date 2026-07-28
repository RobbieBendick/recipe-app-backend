package recipeimport

import "testing"

func TestFindRecipeJSONLD(t *testing.T) {
	html := `
<html><head>
<script type="application/ld+json">
{
  "@context": "https://schema.org",
  "@type": "Recipe",
  "name": "Test Pasta",
  "description": "A simple pasta.",
  "prepTime": "PT10M",
  "cookTime": "PT20M",
  "recipeYield": "4 servings",
  "recipeIngredient": ["200g pasta", "2 tbsp olive oil"],
  "recipeInstructions": [
    {"@type": "HowToStep", "text": "Boil water."},
    {"@type": "HowToStep", "text": "Cook the pasta."}
  ]
}
</script>
</head><body></body></html>`

	recipe := findRecipeJSONLD(html)
	if recipe == nil {
		t.Fatal("expected recipe node")
	}

	out := &Extracted{Ingredients: []string{}, Steps: []string{}}
	applyJSONLD(out, recipe)

	if out.Title != "Test Pasta" {
		t.Fatalf("title=%q", out.Title)
	}
	if out.Description != "A simple pasta." {
		t.Fatalf("description=%q", out.Description)
	}
	if out.PrepMinutes != 10 || out.CookMinutes != 20 {
		t.Fatalf("times prep=%d cook=%d", out.PrepMinutes, out.CookMinutes)
	}
	if out.Servings != 4 {
		t.Fatalf("servings=%d", out.Servings)
	}
	if len(out.Ingredients) != 2 || out.Ingredients[0] != "200g pasta" {
		t.Fatalf("ingredients=%v", out.Ingredients)
	}
	if len(out.Steps) != 2 || out.Steps[0] != "Boil water." {
		t.Fatalf("steps=%v", out.Steps)
	}
}

func TestFindRecipeInGraph(t *testing.T) {
	html := `<script type="application/ld+json">
{"@context":"https://schema.org","@graph":[
  {"@type":"WebPage","name":"Page"},
  {"@type":"Recipe","name":"Graph Recipe","recipeIngredient":["1 egg"]}
]}
</script>`

	recipe := findRecipeJSONLD(html)
	if recipe == nil {
		t.Fatal("expected recipe in @graph")
	}
	out := &Extracted{Ingredients: []string{}, Steps: []string{}}
	applyJSONLD(out, recipe)
	if out.Title != "Graph Recipe" {
		t.Fatalf("title=%q", out.Title)
	}
}

func TestParseISODurationMinutes(t *testing.T) {
	cases := map[string]int{
		"PT15M":    15,
		"PT1H30M":  90,
		"P1DT2H":   1560,
		"PT45S":    1,
		"not-iso":  0,
		"":         0,
	}
	for in, want := range cases {
		if got := parseISODurationMinutes(in); got != want {
			t.Fatalf("%q => %d, want %d", in, got, want)
		}
	}
}

func TestValidateURLRejectsLocal(t *testing.T) {
	if _, err := validateURL("http://localhost/secret"); err != ErrBlockedHost {
		t.Fatalf("expected blocked host, got %v", err)
	}
	if _, err := validateURL("ftp://example.com"); err != ErrInvalidURL {
		t.Fatalf("expected invalid URL, got %v", err)
	}
}

func TestCleanTextUnescapesEntities(t *testing.T) {
	in := "It shouldn&#8217;t be hot &#8211; but &#8220;wet&#8221; dough."
	want := "It shouldn\u2019t be hot \u2013 but \u201cwet\u201d dough."
	if got := cleanText(in); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
