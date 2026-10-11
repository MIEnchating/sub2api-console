package configstore_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/sqliteutil"
)

func jsonSettingsFixture(t *testing.T) (*configstore.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "private.db")
	store, err := configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for _, key := range []string{"first", "second", "unrelated"} {
		if err := store.SaveJSONSetting(context.Background(), key, "0"); err != nil {
			t.Fatal(err)
		}
	}
	return store, path
}

func TestConcurrentJSONSettingsUpdatesRetainEveryIncrementAcrossBothKeys(t *testing.T) {
	store, _ := jsonSettingsFixture(t)
	const writers = 8
	start, results := make(chan struct{}), make(chan error, writers)
	for range writers {
		go func() {
			<-start
			results <- store.UpdateJSONSettings(context.Background(), []string{"first", "second"}, func(values map[string]string) (map[string]string, error) {
				if len(values) != 2 || values["first"] != values["second"] {
					return nil, errors.New("transaction did not read a consistent, scoped snapshot")
				}
				value, err := strconv.Atoi(values["first"])
				if err != nil {
					return nil, err
				}
				values["first"], values["second"] = strconv.Itoa(value+1), strconv.Itoa(value+1)
				return values, nil
			})
		}()
	}
	close(start)
	for range writers {
		if err := <-results; err != nil {
			t.Error(err)
		}
	}
	for _, key := range []string{"first", "second"} {
		value, err := store.JSONSetting(context.Background(), key)
		if err != nil || value != strconv.Itoa(writers) {
			t.Fatalf("lost update for %s: value=%q err=%v", key, value, err)
		}
	}
}

func TestJSONSettingsRejectedChangesLeaveAllValuesIntact(t *testing.T) {
	for _, reason := range []string{"callback error", "out of scope", "oversized", "cancelled"} {
		t.Run(reason, func(t *testing.T) {
			store, _ := jsonSettingsFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			err := store.UpdateJSONSettings(ctx, []string{"first", "second"}, func(values map[string]string) (map[string]string, error) {
				values["first"] = "changed"
				switch reason {
				case "callback error":
					return values, errors.New("rejected")
				case "out of scope":
					values["unrelated"] = "changed"
				case "oversized":
					values["second"] = strings.Repeat("x", (4<<20)+1)
				case "cancelled":
					cancel()
				}
				return values, nil
			})
			if err == nil {
				t.Fatal("invalid update succeeded")
			}
			for _, key := range []string{"first", "second", "unrelated"} {
				value, err := store.JSONSetting(context.Background(), key)
				if err != nil || value != "0" {
					t.Fatalf("rejected update changed %s: value=%q err=%v", key, value, err)
				}
			}
		})
	}
}

func TestJSONSettingsUnchangedOrOmittedValuesAvoidWritesAndMissingKeysCanBeCreated(t *testing.T) {
	store, path := jsonSettingsFixture(t)
	db, err := sqliteutil.OpenDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TRIGGER reject_existing_update BEFORE INSERT ON settings WHEN NEW.key IN ('first','second') BEGIN SELECT RAISE(ABORT, 'unchanged setting written'); END`); err != nil {
		t.Fatal(err)
	}
	err = store.UpdateJSONSettings(context.Background(), []string{"first", "second", "new"}, func(values map[string]string) (map[string]string, error) {
		if !reflect.DeepEqual(values, map[string]string{"first": "0", "second": "0"}) {
			return nil, errors.New("unexpected setting scope")
		}
		return map[string]string{"first": "0", "new": ""}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM settings WHERE key='new' AND value=''`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("missing empty setting was not created: count=%d err=%v", count, err)
	}
}

func TestJSONSettingsInvalidScopeDoesNotInvokeCallback(t *testing.T) {
	store, _ := jsonSettingsFixture(t)
	for _, keys := range [][]string{nil, {" "}, {"first", "first"}} {
		err := store.UpdateJSONSettings(context.Background(), keys, func(map[string]string) (map[string]string, error) {
			t.Error("invalid scope reached callback")
			return nil, nil
		})
		if err == nil {
			t.Fatalf("invalid scope accepted: %v", keys)
		}
	}
	if err := store.UpdateJSONSettings(context.Background(), []string{"first"}, nil); err == nil {
		t.Fatal("nil callback accepted")
	}
}
