package config

import (
	"strings"
	"testing"
)

func TestLoadRejectsInvalidCookieSecureInsteadOfDisablingIt(t *testing.T) {
	t.Setenv("SUB2API_CONSOLE_COOKIE_SECURE", "tru")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "SUB2API_CONSOLE_COOKIE_SECURE") {
		t.Fatalf("invalid secure-cookie setting was accepted: %v", err)
	}
}

func TestLoadAcceptsExplicitSecureCookieSetting(t *testing.T) {
	t.Setenv("SUB2API_CONSOLE_COOKIE_SECURE", "true")
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.CookieSecure {
		t.Fatal("explicit secure-cookie setting was not applied")
	}
}
