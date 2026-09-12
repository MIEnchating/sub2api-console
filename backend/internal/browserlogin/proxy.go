package browserlogin

import (
	"context"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

// Only fixed, pre-resolved HTTPS destinations are reachable through this proxy.
// Unlike page interception, it also covers popups, workers, and literal-IP URLs.
type loginProxy struct {
	server   *http.Server
	listener net.Listener
	mu       sync.Mutex
	tunnels  map[net.Conn]struct{}
	cancel   context.CancelFunc
}

func startLoginProxy(parent context.Context, destinations map[string]string) (*loginProxy, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	p := &loginProxy{listener: listener, tunnels: map[net.Conn]struct{}{}, cancel: cancel}
	p.server = &http.Server{ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 15 * time.Second, MaxHeaderBytes: 8192, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		address, allowed := destinations[r.Host]
		if r.Method != http.MethodConnect || !allowed {
			http.Error(w, "Destination not permitted", http.StatusForbidden)
			return
		}
		upstream, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", address)
		if err != nil {
			http.Error(w, "Upstream unavailable", http.StatusBadGateway)
			return
		}
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			upstream.Close()
			http.Error(w, "Tunnel unavailable", 500)
			return
		}
		client, buffer, err := hijacker.Hijack()
		if err != nil {
			upstream.Close()
			return
		}
		p.mu.Lock()
		if ctx.Err() != nil {
			p.mu.Unlock()
			client.Close()
			upstream.Close()
			return
		}
		p.tunnels[client] = struct{}{}
		p.tunnels[upstream] = struct{}{}
		p.mu.Unlock()
		defer func() {
			client.Close()
			upstream.Close()
			p.mu.Lock()
			delete(p.tunnels, client)
			delete(p.tunnels, upstream)
			p.mu.Unlock()
		}()
		_ = client.SetDeadline(time.Now().Add(Lifetime))
		_ = upstream.SetDeadline(time.Now().Add(Lifetime))
		if _, err = buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
			return
		}
		if err = buffer.Flush(); err != nil {
			return
		}
		done := make(chan struct{})
		go func() { _, _ = io.Copy(upstream, buffer); upstream.Close(); close(done) }()
		_, _ = io.Copy(client, upstream)
		client.Close()
		<-done
	})}
	go func() { _ = p.server.Serve(listener) }()
	go func() { <-ctx.Done(); p.closeConnections() }()
	return p, nil
}
func (p *loginProxy) Address() string { return "http://" + p.listener.Addr().String() }
func (p *loginProxy) closeConnections() {
	_ = p.server.Close()
	p.mu.Lock()
	defer p.mu.Unlock()
	for connection := range p.tunnels {
		_ = connection.Close()
	}
}
func (p *loginProxy) Close() { p.cancel(); p.closeConnections() }
