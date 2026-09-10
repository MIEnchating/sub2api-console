package configstore

import (
	"context"
	"errors"
	"testing"
)

func TestUptimeKumaConfigPreservesSecretsAndRejectsStaleWrites(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	in := UptimeKumaConfig{BaseURL: "https://kuma.example", APIKey: "test-key", Username: "admin", Password: "test-password", Token: "test-session"}
	if err := s.SaveUptimeKuma(ctx, in); err != nil {
		t.Fatal(err)
	}
	got, err := s.UptimeKuma(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Revision != 1 || got.APIKey != in.APIKey || got.Token != in.Token || got.Password != in.Password {
		t.Fatalf("stored configuration did not round trip")
	}
	if err := s.SaveUptimeKuma(ctx, in); !errors.Is(err, ErrKumaConfigConflict) {
		t.Fatalf("stale save: %v", err)
	}
	if err := s.SaveUptimeKuma(ctx, UptimeKumaConfig{Revision: 1}); err != nil {
		t.Fatal(err)
	}
	got, err = s.UptimeKuma(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Revision != 2 || got.APIKey != "" || got.Token != "" || got.Password != "" {
		t.Fatal("disconnect must remove all credentials without resetting revision")
	}
	if err := s.SaveUptimeKuma(ctx, in); !errors.Is(err, ErrKumaConfigConflict) {
		t.Fatalf("stale create after disconnect: %v", err)
	}
}
