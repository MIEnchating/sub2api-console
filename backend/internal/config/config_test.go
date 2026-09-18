package config

import (
	"net/netip"
	"path/filepath"
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

func TestLoadAcceptsTrustedProxyCIDRs(t *testing.T) {
	t.Setenv("SUB2API_CONSOLE_TRUSTED_PROXY_CIDRS", "172.16.0.0/12, 10.0.0.8/32")
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	want := []netip.Prefix{
		netip.MustParsePrefix("172.16.0.0/12"),
		netip.MustParsePrefix("10.0.0.8/32"),
	}
	if len(loaded.TrustedProxyCIDRs) != len(want) {
		t.Fatalf("trusted proxies = %v, want %v", loaded.TrustedProxyCIDRs, want)
	}
	for index := range want {
		if loaded.TrustedProxyCIDRs[index] != want[index] {
			t.Fatalf("trusted proxies = %v, want %v", loaded.TrustedProxyCIDRs, want)
		}
	}
}

func TestLoadRejectsInvalidTrustedProxyCIDR(t *testing.T) {
	t.Setenv("SUB2API_CONSOLE_TRUSTED_PROXY_CIDRS", "172.16.0.0/12,not-a-cidr")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "SUB2API_CONSOLE_TRUSTED_PROXY_CIDRS") {
		t.Fatalf("invalid trusted proxy CIDR was accepted: %v", err)
	}
}

func TestLoadAcceptsAbsoluteTrustedProxySocket(t *testing.T) {
	want := filepath.Join(t.TempDir(), "proxy", "api.sock")
	t.Setenv("SUB2API_CONSOLE_TRUSTED_PROXY_SOCKET", want)
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.TrustedProxySocket != want {
		t.Fatalf("trusted proxy socket = %q, want %q", loaded.TrustedProxySocket, want)
	}
}

func TestLoadRejectsRelativeTrustedProxySocket(t *testing.T) {
	t.Setenv("SUB2API_CONSOLE_TRUSTED_PROXY_SOCKET", "run/api.sock")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "SUB2API_CONSOLE_TRUSTED_PROXY_SOCKET") {
		t.Fatalf("relative trusted proxy socket was accepted: %v", err)
	}
}

func TestLoadAcceptsStrongSetupToken(t *testing.T) {
	t.Setenv("SUB2API_CONSOLE_SETUP_TOKEN", strings.Repeat("a", 32))
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SetupToken != strings.Repeat("a", 32) {
		t.Fatal("setup token was not loaded")
	}
}

func TestLoadRejectsWeakSetupToken(t *testing.T) {
	t.Setenv("SUB2API_CONSOLE_SETUP_TOKEN", "short-token")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "SUB2API_CONSOLE_SETUP_TOKEN") {
		t.Fatalf("weak setup token was accepted: %v", err)
	}
}

func TestLoadRejectsWeakAdminToken(t *testing.T) {
	t.Setenv("SUB2API_CONSOLE_CONSOLE_ADMIN_TOKEN", "short-token")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "SUB2API_CONSOLE_CONSOLE_ADMIN_TOKEN") {
		t.Fatalf("weak admin token was accepted: %v", err)
	}
}

func TestLoadReadsTaskConcurrencyAndQueueCapacity(t *testing.T) {
	t.Setenv("SUB2API_TASK_CONCURRENCY_ACCOUNT", "12")
	t.Setenv("SUB2API_TASK_QUEUE_CAPACITY", "25")
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.TaskConcurrency["account"] != 12 || loaded.TaskQueueCapacity != 25 {
		t.Fatalf("task limits = %#v queue=%d", loaded.TaskConcurrency, loaded.TaskQueueCapacity)
	}
}

func TestLoadRejectsInvalidTaskConcurrency(t *testing.T) {
	t.Setenv("SUB2API_TASK_CONCURRENCY_PROBE", "0")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "SUB2API_TASK_CONCURRENCY_PROBE") {
		t.Fatalf("invalid task concurrency was accepted: %v", err)
	}
}
