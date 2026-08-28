package writebackpolicy

import "testing"

func TestVerificationDefaultsToDisabled(t *testing.T) {
	for _, document := range []map[string]any{nil, {}, {"writeback": map[string]any{}}} {
		value, err := Verification(document)
		if err != nil || value {
			t.Fatalf("verification=%v err=%v", value, err)
		}
	}
}

func TestVerificationReadsAndValidatesPolicy(t *testing.T) {
	value, err := Verification(map[string]any{"writeback": map[string]any{"verification": true}})
	if err != nil || !value {
		t.Fatalf("verification=%v err=%v", value, err)
	}
	if _, err := Verification(map[string]any{"writeback": map[string]any{"verification": "yes"}}); err == nil {
		t.Fatal("invalid verification value was accepted")
	}
}

func TestConcurrencyDefaultsAndValidatesPolicy(t *testing.T) {
	value, err := Concurrency(nil)
	if err != nil || value != 4 {
		t.Fatalf("concurrency=%d err=%v", value, err)
	}
	value, err = Concurrency(map[string]any{"writeback": map[string]any{"concurrency": int64(8)}})
	if err != nil || value != 8 {
		t.Fatalf("concurrency=%d err=%v", value, err)
	}
	if _, err := Concurrency(map[string]any{"writeback": map[string]any{"concurrency": 17}}); err == nil {
		t.Fatal("out-of-range concurrency was accepted")
	}
}
