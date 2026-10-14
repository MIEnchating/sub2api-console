package business_test

import (
	"reflect"
	"testing"
)

func TestProbeBindingPreservesDetectionIdentityAndTypeFallback(t *testing.T) {
	for _, metadata := range []string{
		`{"platform":"openai","account_type":"oauth","base_url":"https://direct.example"}`,
		`{"platform":"anthropic","type":"apikey"}`,
		`{}`,
	} {
		t.Run(metadata, func(t *testing.T) {
			store, db := accountOwnershipFixture(t, 1)
			if _, err := db.Exec(`UPDATE accounts SET metadata_json=? WHERE id='41'`, metadata); err != nil {
				t.Fatal(err)
			}
			detail, err := store.Account(t.Context(), "41")
			if err != nil {
				t.Fatal(err)
			}
			binding, err := store.ProbeAccountBinding(t.Context(), "41")
			if err != nil {
				t.Fatal(err)
			}
			if binding.ID != detail.ID || binding.Name != detail.Name ||
				!reflect.DeepEqual(binding.Platform, detail.Platform) || !reflect.DeepEqual(binding.AccountType, detail.AccountType) ||
				!reflect.DeepEqual(binding.BaseURL, detail.BaseURL) || !reflect.DeepEqual(binding.UpstreamBaseURL, detail.UpstreamBaseURL) ||
				!reflect.DeepEqual(binding.Bindings, detail.Bindings) {
				t.Fatalf("binding identity differs from detail: %+v", binding)
			}
		})
	}
}
