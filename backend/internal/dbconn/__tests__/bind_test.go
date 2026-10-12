package dbconn_test

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"testing"
)

func TestPostgresBindingPreservesQuotedTextAndComments(t *testing.T) {
	query := `SELECT ?, '?', 'it''s ?', "question?", $$?$$, $body$?$body$ /* ? /* ? */ */ -- ?
 WHERE id=? AND payload ?? 'field'`
	want := `SELECT $1, '?', 'it''s ?', "question?", $$?$$, $body$?$body$ /* ? /* ? */ */ -- ?
 WHERE id=$2 AND payload ? 'field'`
	if got := dbconn.Postgres.Bind(query); got != want {
		t.Fatalf("binding changed SQL contents: %s", got)
	}
}

func TestMySQLBindingQuotesIdentifiersWithoutChangingValues(t *testing.T) {
	query := `SELECT "key", 'key?', ? FROM settings WHERE "key"=?`
	want := "SELECT `key`, 'key?', ? FROM settings WHERE `key`=?"
	if got := dbconn.MySQL.Bind(query); got != want {
		t.Fatalf("identifier binding: %s", got)
	}
	if got := dbconn.SQLite.Bind(query); got != query {
		t.Fatalf("SQLite changed: %s", got)
	}
}

func TestBindingPreservesEscapedQuotesAndMalformedIdentifiers(t *testing.T) {
	for _, item := range []struct {
		driver      dbconn.Driver
		query, want string
	}{
		{dbconn.Postgres, `SELECT E'escaped\' ?', ?`, `SELECT E'escaped\' ?', $1`},
		{dbconn.MySQL, `SELECT 'escaped\' ?', "key", ?`, "SELECT 'escaped\\' ?', `key`, ?"},
		{dbconn.MySQL, `"`, `"`},
	} {
		if got := item.driver.Bind(item.query); got != item.want {
			t.Errorf("%s: got %q, want %q", item.driver, got, item.want)
		}
	}
}
