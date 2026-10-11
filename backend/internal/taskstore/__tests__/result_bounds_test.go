package taskstore_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

// The saver is the persistence boundary. Retain its input to inspect the exact
// result that would be written, without copying production truncation logic.
type resultSaver struct {
	task  taskstore.Task
	calls int
}

func (s *resultSaver) Save(_ context.Context, task taskstore.Task) error {
	s.task = task
	s.calls++
	return nil
}

func oversizedResult() map[string]any {
	items := make([]any, 2000)
	for index := range items {
		items[index] = map[string]any{"id": index, "detail": strings.Repeat("x", 6000)}
	}
	return map[string]any{"items": items, "account_id": "41", "precise": json.Number("9007199254740993")}
}

func TestSaveFinalBoundsOversizedResultsWithoutChangingOriginalOrNumberPrecision(t *testing.T) {
	result := oversizedResult()
	saver := &resultSaver{}
	if err := taskstore.SaveFinal(context.Background(), saver, taskstore.Task{Result: result}); err != nil {
		t.Fatal(err)
	}
	saved := saver.task.Result
	items, ok := saved["items"].([]any)
	if !ok || len(items) != 500 || saved["total"] != 2000 || saved["retained"] != 500 || saved["truncated"] != true {
		t.Fatalf("unexpected bounded result: count=%d, total=%v, retained=%v", len(items), saved["total"], saved["retained"])
	}
	if saved["account_id"] != "41" || saved["precise"] != json.Number("9007199254740993") {
		t.Fatal("bounded result changed identity or integer precision")
	}
	if len(result["items"].([]any)) != 2000 || result["truncated"] != nil {
		t.Fatal("bounding mutated the caller's result")
	}
	items[0].(map[string]any)["id"] = "edited saved copy"
	if result["items"].([]any)[0].(map[string]any)["id"] != 0 {
		t.Fatal("bounded result aliases the caller's records")
	}
	raw, err := json.Marshal(saved)
	if err != nil || len(raw) > 4<<20 {
		t.Fatalf("bounded encoding = %d bytes, %v", len(raw), err)
	}
}

func TestSaveFinalKeepsUsableNestedArrayEntriesWhenFullResultExceedsLimit(t *testing.T) {
	items := make([]any, 1500)
	for index := range items {
		items[index] = strings.Repeat("x", 6000)
	}
	saver := &resultSaver{}
	if err := taskstore.SaveFinal(context.Background(), saver, taskstore.Task{Result: map[string]any{"items": []any{items}}}); err != nil {
		t.Fatal(err)
	}
	outer, ok := saver.task.Result["items"].([]any)
	if !ok || len(outer) != 1 {
		t.Fatal("nested arrays fell back to discarding all usable details")
	}
	inner, ok := outer[0].([]any)
	if !ok || len(inner) != 500 || saver.task.Result["total"] != 1500 || saver.task.Result["retained"] != 500 {
		t.Fatalf("nested array was not bounded: length=%d, total=%v", len(inner), saver.task.Result["total"])
	}
	if len(items) != 1500 {
		t.Fatal("bounding changed the caller's nested array")
	}
}

func TestSaveFinalCancelledBeforeCallDoesNotWrite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	saver := &resultSaver{}
	err := taskstore.SaveFinal(ctx, saver, taskstore.Task{Result: map[string]any{"items": []any{}}})
	if !errors.Is(err, context.Canceled) || saver.calls != 0 {
		t.Fatalf("cancelled save: writes=%d, err=%v", saver.calls, err)
	}
}

func TestSaveFinalKeepsSmallResultUntruncatedAndFallsBackForOversizedScalar(t *testing.T) {
	for _, fixture := range []struct {
		name      string
		result    map[string]any
		truncated bool
	}{
		{"small", map[string]any{"items": []any{}, "account_id": "41"}, false},
		{"large scalar", map[string]any{"detail": strings.Repeat("x", 5<<20), "account_id": "41", "error": "保留失败原因"}, true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			saver := &resultSaver{}
			if err := taskstore.SaveFinal(context.Background(), saver, taskstore.Task{Result: fixture.result}); err != nil {
				t.Fatal(err)
			}
			if (saver.task.Result["truncated"] == true) != fixture.truncated || saver.task.Result["account_id"] != "41" {
				t.Fatal("size boundary changed result identity or truncation state")
			}
			if fixture.truncated && saver.task.Result["error"] != "保留失败原因" {
				t.Fatal("fallback discarded the failure cause")
			}
		})
	}
}

func BenchmarkSaveFinalOversizedResult(b *testing.B) {
	result := oversizedResult()
	saver := &resultSaver{}
	b.ReportAllocs()
	for b.Loop() {
		if err := taskstore.SaveFinal(context.Background(), saver, taskstore.Task{Result: result}); err != nil {
			b.Fatal(err)
		}
		if saver.task.Result["retained"] != 500 {
			b.Fatal("result retention changed")
		}
	}
}
