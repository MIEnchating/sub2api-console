package accountworkbench_test

import (
	"encoding/json"
	"errors"
	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"net/http"
	"strings"
	"testing"
)

type rememberedFixture struct {
	service      *accountworkbench.Service
	store        *configstore.Store
	owner, token string
	runner       *observedRunner
	account      map[string]any
	writes       *int
}

func importedRememberedFixture(t *testing.T, login bool) rememberedFixture {
	t.Helper()
	service, store := fixture(t, `{}`)
	owner, token := previewOwner(t, store)
	runner, _ := runTasks(t, service)
	var input string
	if login {
		protocol := &protocolFixture{t: t}
		signedRunInputWithProtocol(t, service, protocol.response, func(*http.Request) error { return nil })
		input = "run@example.test----test-password"
	} else {
		input = signedRunInput(t, service)
	}
	var current map[string]any
	writes := 0
	service.UseTransport(transportFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "isolated.invalid" {
			return nil, errors.New("unexpected host")
		}
		if req.Method == http.MethodGet {
			if strings.HasSuffix(req.URL.Path, "/accounts") {
				items := []any{}
				if current != nil {
					items = append(items, current)
				}
				return upstreamResponse(map[string]any{"data": map[string]any{"items": items, "total": len(items)}}), nil
			}
			return upstreamResponse(map[string]any{"data": current}), nil
		}
		writes++
		var body map[string]any
		decoder := json.NewDecoder(req.Body)
		decoder.UseNumber()
		if err := decoder.Decode(&body); err != nil {
			return nil, err
		}
		if current == nil {
			current = map[string]any{"id": json.Number("41")}
		}
		for key, value := range body {
			current[key] = value
		}
		return upstreamResponse(map[string]any{"data": current}), nil
	}))
	preview, err := service.Preview(t.Context(), owner, accountworkbench.PreviewInput{Action: "import", Content: input})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.Start(t.Context(), owner, accountworkbench.RunConfirmation{ID: preview.ID, Revision: preview.Revision})
	if err != nil {
		t.Fatal(err)
	}
	awaitRun(t, runner)
	run, err = service.Run(t.Context(), owner, run.ID)
	if err != nil || run.Status != "completed" {
		t.Fatalf("import failed: %v %+v", err, run)
	}

	return rememberedFixture{service, store, owner, token, runner, current, &writes}
}
