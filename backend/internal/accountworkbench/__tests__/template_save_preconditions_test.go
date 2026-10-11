package accountworkbench_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestTemplateSaveRejectsLocalConflictsWithoutReadingUnavailableSource(t *testing.T) {
	for _, scenario := range []struct {
		name      string
		id        string
		revision  int64
		sourceID  string
		empty     bool
		wantError string
	}{
		{name: "stale_create", revision: 0, sourceID: "41", wantError: configstore.ErrWorkbenchVersion.Error()},
		{name: "stale_update", id: "saved", revision: 0, sourceID: "41", wantError: configstore.ErrWorkbenchVersion.Error()},
		{name: "empty_library_version", revision: 1, sourceID: "41", empty: true, wantError: configstore.ErrWorkbenchVersion.Error()},
		{name: "missing_template", id: "deleted-template", revision: 1, sourceID: "41", wantError: "待刷新模板不存在"},
		{name: "changed_source", id: "saved", revision: 1, sourceID: "42", wantError: "不能更改模板来源，请创建新模板"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx := context.Background()
			service, _ := fixture(t, `{"data":`+sourceAccount+`}`)
			source, err := service.TemplateSource(ctx, "41")
			if err != nil {
				t.Fatal(err)
			}
			if !scenario.empty {
				saved, err := service.SaveTemplate(ctx, accountworkbench.TemplateInput{Name: "来源模板", SourceID: "41", SourceVersion: source.SourceVersion})
				if err != nil {
					t.Fatal(err)
				}
				if scenario.id == "saved" {
					scenario.id = saved.Items[0].ID
				}
			}
			before, err := service.Templates(ctx)
			if err != nil {
				t.Fatal(err)
			}
			reads := 0
			service.UseTransport(transportFunc(func(*http.Request) (*http.Response, error) {
				reads++
				return nil, errors.New("isolated upstream unavailable")
			}))

			_, saveErr := service.SaveTemplate(ctx, accountworkbench.TemplateInput{
				ID: scenario.id, Name: "不应保存", Revision: scenario.revision,
				SourceID: scenario.sourceID, SourceVersion: source.SourceVersion,
			})
			if reads != 0 {
				t.Errorf("local conflict made %d upstream requests", reads)
			}
			if saveErr == nil || saveErr.Error() != scenario.wantError {
				t.Errorf("save error = %v, want %q", saveErr, scenario.wantError)
			}
			after, err := service.Templates(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(after, before) {
				t.Fatal("rejected save changed the library")
			}
		})
	}
}

func TestTemplateSaveWithCurrentLibraryStillRevalidatesSource(t *testing.T) {
	for _, scenario := range []struct{ name, from, to string }{
		{name: "configuration_changed", from: `"priority":10`, to: `"priority":20`},
		{name: "identity_changed", from: `"user-one"`, to: `"user-two"`},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx := context.Background()
			service, _ := fixture(t, `{"data":`+sourceAccount+`}`)
			source, err := service.TemplateSource(ctx, "41")
			if err != nil {
				t.Fatal(err)
			}
			reads := 0
			service.UseTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
				reads++
				if r.URL.Host != "isolated.invalid" || r.Method != http.MethodGet {
					t.Fatalf("unexpected source request: %s %s", r.Method, r.URL)
				}
				body := `{"data":` + strings.Replace(sourceAccount, scenario.from, scenario.to, 1) + `}`
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
			}))

			_, err = service.SaveTemplate(ctx, accountworkbench.TemplateInput{Name: "新模板", SourceID: "41", SourceVersion: source.SourceVersion})
			if err == nil || err.Error() != "来源配置已变化，请重新读取后保存" {
				t.Fatalf("save error = %v, want changed-source rejection", err)
			}
			if reads != 1 {
				t.Fatalf("source reads = %d, want one fresh verification", reads)
			}
			library, err := service.Templates(ctx)
			if err != nil || library.Revision != 0 || len(library.Items) != 0 {
				t.Fatalf("source conflict created a template: revision=%d, err=%v", library.Revision, err)
			}
		})
	}
}

func TestTemplateSaveRejectsLibraryChangeDuringSourceRead(t *testing.T) {
	ctx := context.Background()
	service, store := fixture(t, `{"data":`+sourceAccount+`}`)
	source, err := service.TemplateSource(ctx, "41")
	if err != nil {
		t.Fatal(err)
	}
	library, err := service.SaveTemplate(ctx, accountworkbench.TemplateInput{Name: "原模板", SourceID: "41", SourceVersion: source.SourceVersion})
	if err != nil {
		t.Fatal(err)
	}
	documents, err := store.WorkbenchDocuments(ctx, "templates:", 2)
	if err != nil || len(documents) != 1 {
		t.Fatalf("isolated template document: count=%d, err=%v", len(documents), err)
	}
	reads := 0
	service.UseTransport(transportFunc(func(*http.Request) (*http.Response, error) {
		reads++
		// Change real storage at the network boundary, after local validation.
		// This bypasses the service lease to exercise the final storage CAS.
		changed := library
		changed.Items = append([]accountworkbench.Template(nil), library.Items...)
		changed.Items[0].Name = "并发保存的模板"
		raw, err := json.Marshal(changed)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = store.SaveWorkbenchDocument(ctx, documents[0].ID, library.Revision, raw); err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"data":` + sourceAccount + `}`)), Header: http.Header{}}, nil
	}))

	_, err = service.SaveTemplate(ctx, accountworkbench.TemplateInput{
		ID: library.Items[0].ID, Name: "过期草稿", Revision: library.Revision,
		SourceID: "41", SourceVersion: source.SourceVersion,
	})
	if !errors.Is(err, configstore.ErrWorkbenchVersion) {
		t.Fatalf("save error = %v, want version conflict", err)
	}
	if reads != 1 {
		t.Fatalf("source reads = %d, want no automatic replay", reads)
	}
	stored, err := service.Templates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Revision != 2 || len(stored.Items) != 1 || stored.Items[0].Name != "并发保存的模板" {
		t.Fatal("stale draft overwrote the concurrent save")
	}
}
