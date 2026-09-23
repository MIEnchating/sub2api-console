package modelcheck_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

const fixtureSecret = "sk-isolated-animation-test-secret"
const fixtureSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 640 400"><circle cx="100" cy="200" r="40"><animateTransform attributeName="transform" type="rotate" from="0 100 200" to="360 100 200" dur="2s" repeatCount="indefinite"/></circle></svg>`

type catalog struct {
	mu           sync.Mutex
	rows         []business.AccountStatus
	details      map[string]*business.AccountDetail
	raw          []byte
	failSave     bool
	plans        []byte
	groupMembers map[string][]string
}

func (c *catalog) Accounts(context.Context) ([]business.AccountStatus, error) { return c.rows, nil }
func (c *catalog) Account(_ context.Context, id string) (*business.AccountDetail, error) {
	return c.details[id], nil
}
func (c *catalog) LoadAnimationConfiguration(context.Context) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.raw...), nil
}
func (c *catalog) SaveAnimationConfiguration(_ context.Context, raw []byte, _ string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failSave {
		return errors.New("isolated storage failure")
	}
	c.raw = append([]byte(nil), raw...)
	return nil
}

type credentials struct{}

func (credentials) AuthRecord(context.Context, string) (*configstore.AuthRecord, error) {
	return nil, errors.New("uncached credentials must not be used")
}
func (credentials) UpstreamKeySecret(_ context.Context, host, key, group string) (*configstore.UpstreamKeySecret, error) {
	return &configstore.UpstreamKeySecret{Host: host, KeyID: key, GroupID: group, Secret: fixtureSecret}, nil
}
func (credentials) SaveUpstreamKeySecret(context.Context, configstore.UpstreamKeySecret) error {
	return errors.New("unexpected write")
}
func (credentials) RevealKey(context.Context, configstore.AuthRecord, string, string) (upstreamsync.CreatedKey, error) {
	return upstreamsync.CreatedKey{}, errors.New("unexpected remote reveal")
}

type taskRecorder struct {
	*taskstore.Store
	terminal chan taskstore.Task
}

func (r *taskRecorder) Save(ctx context.Context, task taskstore.Task) error {
	err := r.Store.Save(ctx, task)
	if err == nil && (task.Status == "succeeded" || task.Status == "failed" || task.Status == "partial" || task.Status == "cancelled") {
		r.terminal <- task
	}
	return err
}

type fixture struct {
	service *modelcheck.Service
	catalog *catalog
	tasks   *taskRecorder
	runner  *taskrunner.Group
}

func setup(t *testing.T, count int, platform string, handler http.HandlerFunc) *fixture {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	store, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	tasks := &taskRecorder{Store: store, terminal: make(chan taskstore.Task, 64)}
	c := &catalog{details: map[string]*business.AccountDetail{}}
	for i := 1; i <= count; i++ {
		id := strconv.Itoa(i)
		group := "7"
		base := server.URL
		row := business.AccountStatus{ID: id, Name: "测试账号 " + id, Platform: &platform, BaseURL: &base}
		c.rows = append(c.rows, row)
		c.details[id] = &business.AccountDetail{AccountStatus: row, Bindings: []business.AccountBinding{{LocalAccountID: id, UpstreamHost: "account-" + id + ".example.invalid", UpstreamKeyID: "91", UpstreamGroupID: &group}}}
	}
	service, err := modelcheck.New(tasks, credentials{}, c, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	runner := taskrunner.New(context.Background())
	service.UseTaskRunner(runner)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := runner.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	return &fixture{service: service, catalog: c, tasks: tasks, runner: runner}
}
func finished(t *testing.T, f *fixture) taskstore.Task {
	t.Helper()
	select {
	case task := <-f.tasks.terminal:
		return task
	case <-time.After(5 * time.Second):
		t.Fatal("task did not complete")
		return taskstore.Task{}
	}
}
func request(ids ...string) modelcheck.AnimationRequest {
	targets := make([]modelcheck.AnimationTarget, len(ids))
	for i, id := range ids {
		targets[i] = modelcheck.AnimationTarget{AccountID: id, Model: "test-model"}
	}
	return modelcheck.AnimationRequest{Targets: targets, TimeoutSeconds: 5}
}
