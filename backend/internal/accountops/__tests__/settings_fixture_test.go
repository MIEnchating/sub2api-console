package accountops_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountops"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type settingsTarget struct{ endpoint string }

func (s settingsTarget) TargetSettings(context.Context) (configstore.TargetSettings, error) {
	return configstore.TargetSettings{BaseURL: s.endpoint, AdminKey: "test-key", TimeoutSeconds: 1}, nil
}

type settingsTasks struct{ last taskstore.Task }

func (s *settingsTasks) Save(_ context.Context, task taskstore.Task) error {
	s.last = task
	return nil
}

type settingsRunner struct{ run func(context.Context) }

func (r *settingsRunner) Go(run func(context.Context)) error {
	r.run = run
	return nil
}

func (r *settingsRunner) GoTask(_ string, run func(context.Context)) error { return r.Go(run) }
func (*settingsRunner) CancelTask(string) bool                             { return false }

type settingsFixture struct {
	service    *accountops.Service
	repository *business.Store
	runner     *settingsRunner
	tasks      *settingsTasks
	requests   atomic.Int32
}

func newSettingsFixture(t *testing.T) *settingsFixture {
	t.Helper()
	f := &settingsFixture{runner: &settingsRunner{}, tasks: &settingsTasks{}}
	path := filepath.Join(t.TempDir(), "accounts.sqlite3")
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	f.repository = repository
	t.Cleanup(func() { _ = repository.Close() })
	if err := repository.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO accounts(id,name,schedulable,priority,load_factor,concurrency,metadata_json,updated_at)
		VALUES('41','test-account',1,20,'1',3,'{}','now')`)
	_ = db.Close()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		f.requests.Add(1)
		http.Error(w, `{"message":"unexpected settings request"}`, http.StatusBadRequest)
	}))
	t.Cleanup(server.Close)
	f.service = accountops.New(settingsTarget{endpoint: server.URL}, repository, f.tasks)
	f.service.UseTaskRunner(f.runner)
	return f
}
