package upstreamsync_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

func TestSub2APIProfileConcurrencyPreservesKnownUnlimitedAndUnknownLimits(t *testing.T) {
	for _, test := range []struct {
		name    string
		profile string
		status  string
		limit   *int64
	}{
		{name: "positive user limit", profile: `{"id":17,"balance":5,"concurrency":12}`, status: "known", limit: concurrencyPointer(12)},
		{name: "explicit zero is unlimited", profile: `{"id":17,"balance":5,"concurrency":0}`, status: "unlimited", limit: concurrencyPointer(0)},
		{name: "missing limit stays unknown", profile: `{"id":17,"balance":5}`, status: "unknown"},
		{name: "null limit stays unknown", profile: `{"id":17,"balance":5,"concurrency":null}`, status: "unknown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader, record := concurrencyReader(t, test.profile, "sub2api")
			observation, err := reader.ReadBalance(t.Context(), record)
			if err != nil {
				t.Fatal(err)
			}
			if observation.ConcurrencyStatus != test.status || observation.ProfileUserID != "17" {
				t.Fatalf("profile observation lost status or user identity: %#v", observation)
			}
			if test.limit == nil {
				if observation.ConcurrencyLimit != nil {
					t.Fatal("unknown concurrency became a usable budget")
				}
			} else if observation.ConcurrencyLimit == nil || *observation.ConcurrencyLimit != *test.limit {
				t.Fatalf("wrong concurrency limit: %#v", observation.ConcurrencyLimit)
			}
		})
	}
}

func TestSub2APIProfileRejectsMalformedConcurrencyWithoutReturningABudget(t *testing.T) {
	for _, value := range []string{`-1`, `1.5`, `true`, `"invalid"`, `9223372036854775808`, `{}`} {
		t.Run(value, func(t *testing.T) {
			reader, record := concurrencyReader(t, `{"id":17,"balance":5,"concurrency":`+value+`}`, "sub2api")
			observation, err := reader.ReadBalance(t.Context(), record)
			if err == nil || observation.ConcurrencyLimit != nil {
				t.Fatal("malformed concurrency was accepted as a user budget")
			}
		})
	}
}

func TestNewAPIProfileDoesNotAdoptSub2APIConcurrencySemantics(t *testing.T) {
	reader, record := concurrencyReader(t, `{"id":17,"balance":5,"concurrency":12}`, "newapi")
	observation, err := reader.ReadBalance(t.Context(), record)
	if err != nil {
		t.Fatal(err)
	}
	if observation.ConcurrencyLimit != nil || observation.ConcurrencyStatus != "" {
		t.Fatal("New API profile created a Sub2API concurrency budget")
	}
}

func concurrencyPointer(value int64) *int64 { return &value }

func concurrencyReader(t *testing.T, profile, platform string) (*upstreamsync.Reader, configstore.AuthRecord) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/user/profile" || r.URL.Path == "/api/user/self" {
			if r.Header.Get("Authorization") != "Bearer fixture-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"code":0,"data":` + profile + `}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
	}))
	t.Cleanup(server.Close)
	token := "fixture-token"
	return upstreamsync.NewReader(server.Client()), configstore.AuthRecord{
		Host: "fixture.example", BaseURL: server.URL, UpstreamType: platform, AccessToken: &token,
	}
}
