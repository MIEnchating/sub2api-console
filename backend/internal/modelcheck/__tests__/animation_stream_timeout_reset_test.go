package modelcheck_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestAutoQualityContinuesBeyondConfiguredTotalTimeWhileEachReplyArrivesBeforeTimeout(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("unexpected network request") })
	if _, err := f.service.SaveAnimationQualitySettings(t.Context(), modelcheck.AnimationQualitySettings{Enabled: true, AccountID: "1", Model: "review", TimeoutSeconds: 5}, "operator"); err != nil {
		t.Fatal(err)
	}
	original := http.DefaultTransport
	http.DefaultTransport = qualityQueueTransport(func(r *http.Request) (*http.Response, error) {
		var body struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, err
		}
		if body.Model != "review" {
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"output_text":%q}`, htmlAnimation)))}, nil
		}
		reader, writer := io.Pipe()
		context.AfterFunc(r.Context(), func() { writer.CloseWithError(r.Context().Err()) })
		go func() {
			time.Sleep(3 * time.Second)
			fmt.Fprintf(writer, "data: {\"type\":\"response.output_text.delta\",\"delta\":%q}\n\n", validQualityReview)
			time.Sleep(3 * time.Second)
			io.WriteString(writer, "data: {\"type\":\"response.output_text.delta\",\"delta\":\" \"}\n\n")
			time.Sleep(3 * time.Second)
			io.WriteString(writer, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
		}()
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: reader}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = original })
	synctest.Test(t, func(t *testing.T) {
		defer func() {
			if err := f.runner.Shutdown(context.Background()); err != nil {
				t.Error(err)
			}
		}()
		f.tasks.terminal = make(chan taskstore.Task, 1)
		if _, err := f.service.EnqueueAnimation(t.Context(), request("1")); err != nil {
			t.Fatal(err)
		}
		task := finishedWithin(t, f, 12*time.Second)
		row := task.Result["animations"].([]modelcheck.AnimationResult)[0]
		if row.Quality == nil || row.Quality.Status != "normal" || row.Status != "succeeded" {
			t.Fatalf("stream was cut off despite regular replies: %+v", row)
		}
	})
}
