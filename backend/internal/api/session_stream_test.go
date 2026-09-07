package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/gin-gonic/gin"
)

type sessionStreamRecorder struct {
	*httptest.ResponseRecorder
	afterFlush func()
}

func (recorder *sessionStreamRecorder) Flush() {
	recorder.ResponseRecorder.Flush()
	recorder.afterFlush()
}

type sessionStreamInspection struct {
	fakeInspectionController
	updates chan struct{}
}

func (controller sessionStreamInspection) Subscribe() (<-chan struct{}, func()) {
	return controller.updates, func() {}
}

func TestSSEStopsSendingBusinessDataAfterSessionInvalidation(t *testing.T) {
	for _, stream := range []string{"task", "inspection"} {
		for _, invalidation := range []string{"revoked", "expired"} {
			t.Run(stream+"/"+invalidation, func(t *testing.T) {
				_, private := testRouter(t, config.Config{}, fakeBusiness{})
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if err := private.Initialize(ctx, "operator", "test-password", "https://admin.example", "test-key"); err != nil {
					t.Fatal(err)
				}
				now := time.Now().UTC()
				token, err := private.CreateSession(ctx, "operator", time.Hour, now)
				if err != nil {
					t.Fatal(err)
				}
				updates := make(chan struct{}, 1)
				server := &Server{
					private: private, now: func() time.Time { return now },
					tasks:      fakeTaskRepository{rows: []taskstore.Task{{ID: "task-1", Status: "running", Result: map[string]any{}}}},
					inspection: sessionStreamInspection{updates: updates},
				}
				router := gin.New()
				router.Use(server.authorize())
				path, eventMarker := "/tasks/task-1/events", `"id":"task-1"`
				router.GET("/tasks/:task_id/events", server.taskEvents)
				router.GET("/inspection/events", server.autoInspectionEvents)
				if stream == "inspection" {
					path, eventMarker = "/inspection/events", "event: status"
				}
				flushes := 0
				recorder := &sessionStreamRecorder{ResponseRecorder: httptest.NewRecorder()}
				recorder.afterFlush = func() {
					flushes++
					if flushes > 1 {
						cancel()
						return
					}
					if invalidation == "expired" {
						now = now.Add(2 * time.Hour)
					} else if err := private.RevokeSession(ctx, token); err != nil {
						t.Fatal(err)
					}
					updates <- struct{}{}
				}
				request := httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx)
				request.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})

				router.ServeHTTP(recorder, request)

				if recorder.Code != http.StatusOK || strings.Count(recorder.Body.String(), eventMarker) != 1 {
					t.Fatalf("stream continued after session %s: status=%d body=%s", invalidation, recorder.Code, recorder.Body.String())
				}
			})
		}
	}
}
