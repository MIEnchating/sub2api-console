package accountworkbench_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func BenchmarkRunRecordDecoding(b *testing.B) {
	for _, size := range []int{1024, 128 << 10, 1 << 20} {
		b.Run(fmt.Sprintf("private_bytes=%d", size), func(b *testing.B) {
			store, err := configstore.Open(filepath.Join(b.TempDir(), "records.db"))
			if err != nil {
				b.Fatal(err)
			}
			b.Cleanup(func() { _ = store.Close() })
			token, err := store.CreateSession(b.Context(), "benchmark-admin", time.Hour, time.Now().UTC())
			if err != nil {
				b.Fatal(err)
			}
			hash := sha256.Sum256([]byte(token))
			owner := hex.EncodeToString(hash[:])
			id := strings.Repeat("a", 32)
			raw, err := json.Marshal(map[string]any{"owner": owner, "public": accountworkbench.Run{ID: id, Action: "export", Status: "completed"}, "exports": []any{map[string]any{"private": strings.Repeat("x", size)}}})
			if err != nil {
				b.Fatal(err)
			}
			if _, err := store.SaveWorkbenchDocument(b.Context(), "run:"+owner+":"+id, 0, raw); err != nil {
				b.Fatal(err)
			}
			service := accountworkbench.New(store)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				run, err := service.Run(b.Context(), owner, id)
				if err != nil || run.ID != id || run.Revision != 1 {
					b.Fatal("record read failed", err)
				}
			}
		})
	}
}

func BenchmarkTemplatePayloadDecoding(b *testing.B) {
	for _, count := range []int{1, 1000} {
		b.Run(fmt.Sprintf("models=%d", count), func(b *testing.B) {
			mapping := make(map[string]string, count)
			for i := range count {
				mapping[fmt.Sprintf("requested-model-%04d", i)] = fmt.Sprintf("returned-model-%04d", i)
			}
			raw, err := json.Marshal(mapping)
			if err != nil {
				b.Fatal(err)
			}
			template := &accountworkbench.Template{Config: accountworkbench.TemplateConfig{Concurrency: 1, Priority: 50, RateMultiplier: "0.1234567890123456789", Notes: strings.Repeat("n", 16384), CredentialExtras: map[string]json.RawMessage{"model_mapping": raw}}}
			item := accountworkbench.InputItem{Credentials: map[string]any{"access_token": "isolated-access", "chatgpt_account_id": "workspace", "chatgpt_user_id": "user"}}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				payload, err := accountworkbench.AccountPayload(item, template)
				if err != nil || payload["rate_multiplier"] != json.Number("0.1234567890123456789") {
					b.Fatal("template payload failed", err)
				}
			}
		})
	}
}

func BenchmarkInputJWTDecoding(b *testing.B) {
	for _, size := range []int{128, 8192} {
		b.Run(fmt.Sprintf("claim_padding=%d", size), func(b *testing.B) {
			claims := `{"email":"hint@example.test","padding":"` + strings.Repeat("x", size) + `","https://api.openai.com/auth":{"chatgpt_account_id":"workspace","chatgpt_user_id":"user"}}`
			token := "header." + base64.RawURLEncoding.EncodeToString([]byte(claims)) + ".signature"
			raw, err := json.Marshal(map[string]any{"access_token": token})
			if err != nil {
				b.Fatal(err)
			}
			input := string(raw)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				parsed := accountworkbench.ParseInput(input, false)
				if len(parsed.Errors) != 0 || len(parsed.Items) != 1 || parsed.Items[0].IdentitySource != "jwt_claims_unverified" {
					b.Fatal("input decoding failed", parsed.Errors)
				}
			}
		})
	}
}
