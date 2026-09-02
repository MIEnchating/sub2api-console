package main

import (
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestHTTPServerDoesNotTerminateLongLivedSSEWrites(t *testing.T) {
	server := newHTTPServer("127.0.0.1:0", http.NewServeMux())
	if server.WriteTimeout != 0 {
		t.Fatalf("write timeout = %s; SSE connections require no server-wide write deadline", server.WriteTimeout)
	}
	if server.ReadHeaderTimeout != 10*time.Second || server.ReadTimeout != 30*time.Second || server.IdleTimeout != 60*time.Second {
		t.Fatalf("unexpected HTTP timeout configuration: %#v", server)
	}
}

func TestTrustedProxySocketRejectsOccupiedPathsAndReplacesStaleSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "proxy", "api.sock")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("do not overwrite"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := listenTrustedProxySocket(path); err == nil {
		t.Fatal("regular file at socket path was overwritten")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	target := path + ".target"
	if err := os.WriteFile(target, []byte("symlink target"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, err := listenTrustedProxySocket(path); err == nil {
		t.Fatal("symlink at socket path was overwritten")
	}
	if info, err := os.Lstat(path); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("socket-path symlink was changed: info=%v err=%v", info, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	active, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := listenTrustedProxySocket(path); err == nil || errors.Is(err, syscall.ECONNREFUSED) {
		_ = active.Close()
		t.Fatalf("externally managed active socket was not rejected: %v", err)
	}
	if err := active.Close(); err != nil {
		t.Fatal(err)
	}

	stale, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	stale.SetUnlinkOnClose(false)
	if err := stale.Close(); err != nil {
		t.Fatal(err)
	}
	listener, err := listenTrustedProxySocket(path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSocket == 0 || info.Mode().Perm() != 0o660 {
		t.Fatalf("socket mode = %v", info.Mode())
	}
	if _, err := listenTrustedProxySocket(path); err == nil || errors.Is(err, syscall.ECONNREFUSED) {
		t.Fatalf("active socket was not rejected: %v", err)
	}
}

func TestTrustedProxySocketConcurrentStaleCleanupHasOneWinner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "proxy", "api.sock")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	stale, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	stale.SetUnlinkOnClose(false)
	if err := stale.Close(); err != nil {
		t.Fatal(err)
	}

	type result struct {
		listener net.Listener
		err      error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	for range 2 {
		go func() {
			<-start
			listener, err := listenTrustedProxySocket(path)
			results <- result{listener: listener, err: err}
		}()
	}
	close(start)

	var winner net.Listener
	for range 2 {
		result := <-results
		if result.err == nil {
			if winner != nil {
				_ = result.listener.Close()
				_ = winner.Close()
				t.Fatal("concurrent starts both replaced the stale socket")
			}
			winner = result.listener
		}
	}
	if winner == nil {
		t.Fatal("concurrent starts had no winner")
	}
	defer winner.Close()
	assertUnixSocketReachable(t, path)
}

func TestStaleSocketCleanupPreservesSocketReplacedAfterInspection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "api.sock")
	stale, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	stale.SetUnlinkOnClose(false)
	if err := stale.Close(); err != nil {
		t.Fatal(err)
	}
	inspected, err := openSocketIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	defer inspected.Close()
	if connection, err := net.DialTimeout("unix", path, 200*time.Millisecond); err == nil {
		_ = connection.Close()
		t.Fatal("closed socket unexpectedly accepted a connection")
	} else if !errors.Is(err, syscall.ECONNREFUSED) {
		t.Fatalf("closed socket dial error = %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	replacement, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer replacement.Close()

	removed, err := removeSocketIfSame(path, inspected)
	if err != nil {
		t.Fatal(err)
	}
	if removed {
		t.Fatal("stale cleanup removed the replacement socket")
	}
	assertUnixSocketReachable(t, path)
}

func TestTrustedProxySocketClosePreservesReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "proxy", "api.sock")
	listener, err := listenTrustedProxySocket(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		_ = listener.Close()
		t.Fatal(err)
	}
	replacement, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		_ = listener.Close()
		t.Fatal(err)
	}
	defer replacement.Close()

	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	assertUnixSocketReachable(t, path)
}

func TestTrustedProxySocketCloseRemovesOwnedPathAndReleasesLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "proxy", "api.sock")
	listener, err := listenTrustedProxySocket(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owned socket remains after close: %v", err)
	}

	restarted, err := listenTrustedProxySocket(path)
	if err != nil {
		t.Fatalf("listen after close: %v", err)
	}
	if err := restarted.Close(); err != nil {
		t.Fatal(err)
	}
}

func assertUnixSocketReachable(t *testing.T, path string) {
	t.Helper()
	connection, err := net.DialTimeout("unix", path, time.Second)
	if err != nil {
		t.Fatalf("dial unix socket: %v", err)
	}
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}
}
