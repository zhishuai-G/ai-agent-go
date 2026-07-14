package schema

import "testing"

func TestGenerate(t *testing.T) {
	type weatherArgs struct {
		City    string    `json:"city" desc:"城市名"`
		Days    int       `json:"days,omitempty" desc:"预报天数"`
		Scores  []float64 `json:"scores"`
		Ignored string    `json:"-"`
	}

	generated := Generate(weatherArgs{})
	if generated == nil || generated.Type != "object" {
		t.Fatalf("schema = %#v", generated)
	}
	if generated.Properties["city"].Type != "string" || generated.Properties["city"].Description != "城市名" {
		t.Fatalf("city = %#v", generated.Properties["city"])
	}
	if generated.Properties["scores"].Type != "array" || generated.Properties["scores"].Items.Type != "number" {
		t.Fatalf("scores = %#v", generated.Properties["scores"])
	}
	if _, exists := generated.Properties["Ignored"]; exists {
		t.Fatal("ignored field is present")
	}
	if len(generated.Required) != 2 || generated.Required[0] != "city" || generated.Required[1] != "scores" {
		t.Fatalf("required = %#v", generated.Required)
	}
}

func TestGenerateNil(t *testing.T) {
	if got := Generate(nil); got != nil {
		t.Fatalf("Generate(nil) = %#v", got)
	}
}
