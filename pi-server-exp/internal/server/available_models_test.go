package server

import "testing"

func TestParseAvailableModels(t *testing.T) {
	output := `startup warning
provider               model                          context  max-out  thinking  images
openai                 gpt-5.4                        1M       128K     yes       yes
fireworks              accounts/fireworks/models/kimi 262K     32K      yes       no
openai                 gpt-5.4                        1M       128K     yes       yes
`
	models := parseAvailableModels(output)
	if len(models) != 2 {
		t.Fatalf("models = %#v, want 2", models)
	}
	if models[0].Provider != "openai" || models[0].ID != "gpt-5.4" || models[0].Name != "gpt-5.4" {
		t.Fatalf("first model = %#v", models[0])
	}
	if models[1].Provider != "fireworks" || models[1].ID != "accounts/fireworks/models/kimi" || models[1].Name != "kimi" {
		t.Fatalf("second model = %#v", models[1])
	}
}

func TestParseAvailableModelsRequiresHeader(t *testing.T) {
	if models := parseAvailableModels("warning with two words\n"); len(models) != 0 {
		t.Fatalf("models = %#v, want none", models)
	}
}
