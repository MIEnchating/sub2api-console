package business

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"testing"
)

func TestAccountModelSyncPreviewKeepsCatalogsPerAccountAndAppliesGlobalBlockPatterns(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	for _, statement := range []string{
		`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES
			('41','account-a','{"known_models":["model-a","model-c","model-d"],"platform":"anthropic"}','now'),
			('42','account-b','{"known_models":["model-a","model-b","model-c","model-d","model-e"],"platform":"openai"}','now')`,
	} {
		if _, err := store.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.SaveAccountModelExclusionList(ctx, []string{" model-e ", "MODEL-E", "*-image-*"}, "operator"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAccountTestModels(ctx, "42", []string{"model-b"}, "operator"); err != nil {
		t.Fatal(err)
	}

	preview, err := store.AccountModelSyncPreview(ctx, []string{"42", "41"})
	if err != nil {
		t.Fatal(err)
	}
	if preview.AccountCount != 2 || preview.AccountsWithCatalog != 2 || preview.Fingerprint == "" {
		t.Fatalf("preview summary=%#v", preview)
	}
	if !slices.Equal(preview.BlockedPatterns, []string{"*-image-*", "model-e"}) {
		t.Fatalf("blocked patterns=%#v", preview.BlockedPatterns)
	}
	if !slices.Equal(preview.BlockedModels, []string{"model-e"}) {
		t.Fatalf("blocked models=%#v", preview.BlockedModels)
	}
	want := []AccountModelCoverage{
		{Model: "model-a", AccountCount: 2},
		{Model: "model-b", AccountCount: 1},
		{Model: "model-c", AccountCount: 2},
		{Model: "model-d", AccountCount: 2},
		{Model: "model-e", AccountCount: 1},
	}
	if !slices.Equal(preview.Models, want) {
		t.Fatalf("model coverage=%#v", preview.Models)
	}
	wantAccounts := []AccountModelSyncAccount{
		{AccountID: "42", AccountName: "account-b", Platform: "openai", Models: []string{"model-a", "model-b", "model-c", "model-d", "model-e"}, ProbeModel: "model-b"},
		{AccountID: "41", AccountName: "account-a", Platform: "anthropic", Models: []string{"model-a", "model-c", "model-d"}, ProbeModel: ""},
	}
	if !reflect.DeepEqual(preview.Accounts, wantAccounts) {
		t.Fatalf("account previews=%#v", preview.Accounts)
	}
	catalogs, err := store.AccountModelCatalogs(ctx, []string{"41", "42"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(catalogs[0].Models, []string{"model-a", "model-c", "model-d"}) ||
		!slices.Equal(catalogs[1].Models, []string{"model-a", "model-b", "model-c", "model-d", "model-e"}) {
		t.Fatalf("account catalogs=%#v", catalogs)
	}
}

func TestModelMatchesBlockPatternsSupportsCaseInsensitiveStarAndQuestionWildcards(t *testing.T) {
	patterns := []string{"claude-*", "gpt-?.?-preview", "EXACT-MODEL"}
	for _, model := range []string{"Claude-Opus-4", "gpt-5.2-preview", "exact-model"} {
		if !ModelMatchesBlockPatterns(model, patterns) {
			t.Fatalf("model %q did not match %#v", model, patterns)
		}
	}
	for _, model := range []string{"claude", "gpt-5.20-preview", "exact-model-plus"} {
		if ModelMatchesBlockPatterns(model, patterns) {
			t.Fatalf("model %q unexpectedly matched %#v", model, patterns)
		}
	}
}

func TestAccountModelSyncPreviewFingerprintChangesWithCatalog(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,metadata_json,updated_at)
		VALUES('41','account-a','{"known_models":["model-a"]}','now')`); err != nil {
		t.Fatal(err)
	}
	before, err := store.AccountModelSyncPreview(ctx, []string{"41"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveAccountModels(ctx, "41", []string{"model-a", "model-b"}); err != nil {
		t.Fatal(err)
	}
	after, err := store.AccountModelSyncPreview(ctx, []string{"41"})
	if err != nil {
		t.Fatal(err)
	}
	if before.Fingerprint == after.Fingerprint {
		t.Fatal("catalog change did not change preview fingerprint")
	}
}

func TestAccountModelSyncPreviewFingerprintIgnoresDisplayMetadata(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,metadata_json,updated_at)
		VALUES('41','account-a','{"known_models":["model-a"],"platform":"openai"}','now')`); err != nil {
		t.Fatal(err)
	}
	before, err := store.AccountModelSyncPreview(ctx, []string{"41"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE accounts SET name='renamed-account',
		metadata_json='{"known_models":["model-a"],"platform":"anthropic"}' WHERE id='41'`); err != nil {
		t.Fatal(err)
	}
	after, err := store.AccountModelSyncPreview(ctx, []string{"41"})
	if err != nil {
		t.Fatal(err)
	}
	if before.Fingerprint != after.Fingerprint {
		t.Fatal("display-only account metadata changed the model catalog fingerprint")
	}
}

func TestAccountModelSyncPreviewRejectsUnknownAndDuplicateAccounts(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,metadata_json,updated_at)
		VALUES('41','account-a','{}','now')`); err != nil {
		t.Fatal(err)
	}
	for _, ids := range [][]string{{"41", "41"}, {"41", "42"}, {"invalid"}} {
		if _, err := store.AccountModelSyncPreview(ctx, ids); err == nil {
			t.Fatalf("invalid account IDs accepted: %#v", ids)
		}
	}
}

func TestSaveAccountEnabledModelsPersistsOnlyAppliedModels(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,metadata_json,updated_at)
		VALUES('41','account-a','{"known_models":["model-a","model-e"],"platform":"openai"}','now')`); err != nil {
		t.Fatal(err)
	}

	if err := store.SaveAccountEnabledModels(ctx, "41", []string{" model-a ", "MODEL-A"}); err != nil {
		t.Fatal(err)
	}

	var rawMetadata string
	if err := store.db.QueryRowContext(ctx, `SELECT metadata_json FROM accounts WHERE id='41'`).Scan(&rawMetadata); err != nil {
		t.Fatal(err)
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(rawMetadata), &metadata); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(metadataStringList(metadata["known_models"]), []string{"model-a", "model-e"}) {
		t.Fatalf("known models were overwritten: %#v", metadata)
	}
	if !slices.Equal(metadataStringList(metadata["enabled_models"]), []string{"model-a"}) {
		t.Fatalf("enabled models=%#v", metadata["enabled_models"])
	}
	if metadata["platform"] != "openai" || metadata["enabled_models_synced_at"] == nil {
		t.Fatalf("unrelated metadata or sync timestamp missing: %#v", metadata)
	}
}
