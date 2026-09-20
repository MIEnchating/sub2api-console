package accountworkbench_test

import (
	"encoding/json"
	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"testing"
)

func TestSelectedTemplateDoesNotInheritImportedModelMappings(t *testing.T) {
	for _, configured := range []bool{false, true} {
		name := "empty template clears source mappings"
		if configured {
			name = "explicit template replaces source mappings"
		}
		t.Run(name, func(t *testing.T) {
			input := accountworkbench.ParseInput(`{"credentials":{"access_token":"private-access","model_mapping":{"*":"old-model"},"compact_model_mapping":{"*":"old-compact"}}}`, false).Items[0]
			config := accountworkbench.TemplateConfig{Concurrency: 1, RateMultiplier: "1", CredentialExtras: map[string]json.RawMessage{}}
			if configured {
				config.CredentialExtras["model_mapping"] = json.RawMessage(`{"selected":"selected"}`)
			}
			payload, err := accountworkbench.AccountPayload(input, &accountworkbench.Template{Config: config})
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(payload["credentials"])
			var credentials struct {
				Mapping map[string]string `json:"model_mapping"`
				Compact map[string]string `json:"compact_model_mapping"`
			}
			if err := json.Unmarshal(raw, &credentials); err != nil {
				t.Fatal(err)
			}
			if credentials.Mapping["*"] != "" || credentials.Compact["*"] != "" {
				t.Fatal("source model mapping leaked into template selection")
			}
			if configured && (len(credentials.Mapping) != 1 || credentials.Mapping["selected"] != "selected") {
				t.Fatal("selected model mapping changed")
			}
			if input.Credentials["model_mapping"] == nil {
				t.Fatal("original import mutated")
			}
		})
	}
}
