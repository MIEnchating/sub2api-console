package main

import (
	"net/http"
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
