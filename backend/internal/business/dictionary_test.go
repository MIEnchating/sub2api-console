package business

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestDictionaryCatalogReadsIdentitiesWithoutRuntimePolicy(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "catalog.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	for _, statement := range []string{
		`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES ('1','a','{"platform":" OpenAI "}',''),('2','b','{"platform":"OpenAI"}',''),('3','c','{}',''),('4','d','invalid','')`,
		`INSERT INTO local_groups(name,remote_id,updated_at) VALUES ('主分组','42',''),('未同步',NULL,'')`,
	} {
		if _, err := store.db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	for kind, want := range map[string][]configstore.DictionaryEntry{
		"platform": {{Name: "OpenAI", Value: "OpenAI"}},
		"group":    {{Name: "主分组", Value: "42"}},
	} {
		got, err := store.DictionaryValues(context.Background(), kind)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("%s = %v, error = %v", kind, got, err)
		}
	}
}
