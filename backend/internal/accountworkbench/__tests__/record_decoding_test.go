package accountworkbench_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestRunReadPreservesCheckNumbersWithoutExposingPrivateExecution(t *testing.T) {
	service, store := fixture(t, `{}`)
	owner, _ := previewOwner(t, store)
	id := strings.Repeat("a", 32)
	raw := json.RawMessage(`{"owner":"` + owner + `","public":{"id":"` + id + `","action":"import","status":"completed","items":[{"id":"item","check":{"tokens":9007199254740993,"cost":0.1234567890123456789,"nested":[18446744073709551615]}}]},"exports":[{"secret":"private-export"}],"items":[{"credentials":{"refresh_token":"private-token"}}]}`)
	if _, err := store.SaveWorkbenchDocument(t.Context(), "run:"+owner+":"+id, 0, raw); err != nil {
		t.Fatal(err)
	}
	run, err := service.Run(t.Context(), owner, id)
	if err != nil {
		t.Fatal(err)
	}
	check := run.Items[0].Check
	if check["tokens"] != json.Number("9007199254740993") || check["cost"] != json.Number("0.1234567890123456789") || check["nested"].([]any)[0] != json.Number("18446744073709551615") {
		t.Fatal("check numbers lost precision")
	}
	public, err := json.Marshal(run)
	if err != nil || bytes.Contains(public, []byte("private-")) || bytes.Contains(public, []byte(owner)) {
		t.Fatal("public record exposed execution data", err)
	}
	saved, revision, err := store.WorkbenchDocument(t.Context(), "run:"+owner+":"+id)
	if err != nil || revision != run.Revision || !bytes.Equal(raw, saved) {
		t.Fatal("reading changed private record", err)
	}
}

func TestRunReadRejectsInvalidRecordBindingAndPrivateFieldType(t *testing.T) {
	for _, scenario := range []struct {
		name    string
		change  map[string]any
		binding bool
	}{
		{"different owner", map[string]any{"owner": "other"}, true},
		{"different public ID", map[string]any{"public": accountworkbench.Run{ID: strings.Repeat("b", 32)}}, true},
		{"maintenance record", map[string]any{"public": accountworkbench.Run{ID: strings.Repeat("a", 32), Action: "maintenance"}}, true},
		{"invalid private items", map[string]any{"items": "invalid"}, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			service, store := fixture(t, `{}`)
			owner, _ := previewOwner(t, store)
			id := strings.Repeat("a", 32)
			value := map[string]any{"owner": owner, "public": accountworkbench.Run{ID: id}}
			for key, field := range scenario.change {
				value[key] = field
			}
			raw, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.SaveWorkbenchDocument(t.Context(), "run:"+owner+":"+id, 0, raw); err != nil {
				t.Fatal(err)
			}
			_, err = service.Run(t.Context(), owner, id)
			if scenario.binding {
				if !errors.Is(err, accountworkbench.ErrRun) {
					t.Fatal("invalid binding accepted", err)
				}
			} else if err == nil || err.Error() != "处理记录无法读取，请重新输入账号资料" {
				t.Fatal("invalid private field accepted", err)
			}
		})
	}
}
