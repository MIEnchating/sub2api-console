package business_test

import (
	"reflect"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestPlatformDictionaryIncludesGroupOnlyPlatforms(t *testing.T) {
	store, db := pricingProjectionFixture(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES
		('41','普通账号','{"platform":"openai"}','now');
		INSERT INTO local_groups(name,remote_id,platform,updated_at) VALUES
		('国产平价','32','composite','now'),('国产特价','35',' composite ','now'),
		('普通分组','6','openai','now'),('空平台','7',NULL,'now'),
		('空白平台','8','  ','now'),('未同步',NULL,'unbound','now')`); err != nil {
		t.Fatal(err)
	}
	got, err := store.DictionaryValues(t.Context(), "platform")
	if err != nil {
		t.Fatal(err)
	}
	want := []configstore.DictionaryEntry{
		{Name: "composite", Value: "composite"},
		{Name: "openai", Value: "openai"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("platform dictionary = %+v, want %+v", got, want)
	}
}

func TestPlatformDictionaryKeepsGroupPlatformWithoutAccounts(t *testing.T) {
	store, db := pricingProjectionFixture(t)
	if _, err := db.Exec(`INSERT INTO local_groups(name,remote_id,platform,updated_at)
		VALUES ('国产分组','32','composite','now')`); err != nil {
		t.Fatal(err)
	}
	got, err := store.DictionaryValues(t.Context(), "platform")
	want := []configstore.DictionaryEntry{{Name: "composite", Value: "composite"}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("platform dictionary = %+v, error = %v, want %+v", got, err, want)
	}
}
