package browserlogin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type worker struct {
	ctx     context.Context
	factory Factory
	mu      sync.Mutex
	id      string
	browser Browser
	cancel  context.CancelFunc
}

func RunWorker(ctx context.Context, socket string, factory Factory) error {
	_ = os.Remove(socket)
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}
	defer listener.Close()
	defer os.Remove(socket)
	if err = os.Chmod(socket, 0660); err != nil {
		return err
	}
	w := &worker{ctx: ctx, factory: factory}
	defer w.close("")
	server := &http.Server{Handler: w, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 50 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
	go func() { <-ctx.Done(); _ = server.Close() }()
	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
func (w *worker) close(id string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if id != "" && w.id != id {
		return
	}
	if w.cancel != nil {
		w.cancel()
	}
	if w.browser != nil {
		w.browser.Close()
	}
	w.browser = nil
	w.cancel = nil
	w.id = ""
}
func (w *worker) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	response.Header().Set("Content-Type", "application/json")
	response.Header().Set("Cache-Control", "no-store")
	value, err := w.handle(request)
	if err != nil {
		response.WriteHeader(http.StatusConflict)
		value = wireResponse{Error: err.Error()}
	}
	_ = json.NewEncoder(response).Encode(value)
}
func (w *worker) handle(r *http.Request) (wireResponse, error) {
	if r.URL.Path == "/sessions" && r.Method == http.MethodPost {
		var record configstore.AuthRecord
		if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 16384)).Decode(&record); err != nil {
			return wireResponse{}, errors.New("浏览器启动参数无效")
		}
		w.mu.Lock()
		defer w.mu.Unlock()
		if w.id != "" {
			return wireResponse{}, errors.New("验证浏览器正在使用中")
		}
		ctx, cancel := context.WithTimeout(w.ctx, Lifetime)
		// If startup is abandoned, abort it instead of keeping an orphan browser.
		stop := context.AfterFunc(r.Context(), cancel)
		b, err := w.factory.Open(ctx, record)
		stop()
		if err != nil {
			cancel()
			return wireResponse{}, err
		}
		raw := make([]byte, 24)
		if _, err = rand.Read(raw); err != nil {
			cancel()
			b.Close()
			return wireResponse{}, err
		}
		id := hex.EncodeToString(raw)
		w.id = id
		w.browser = b
		w.cancel = cancel
		go func() { <-ctx.Done(); w.close(id) }()
		return wireResponse{ID: id}, nil
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 2 || len(parts) > 3 || parts[0] != "sessions" {
		return wireResponse{}, ErrSession
	}
	if r.Method == http.MethodDelete && len(parts) == 2 {
		w.close(parts[1])
		return wireResponse{}, nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.id != parts[1] || w.browser == nil {
		return wireResponse{}, ErrSession
	}
	if len(parts) == 2 && r.Method == http.MethodGet {
		image, err := w.browser.Screenshot(r.Context())
		return wireResponse{Image: image}, err
	}
	if len(parts) == 3 && r.Method == http.MethodPost {
		switch parts[2] {
		case "input":
			var input Input
			if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 32768)).Decode(&input); err != nil {
				return wireResponse{}, errors.New("浏览器操作参数无效")
			}
			if err := input.Validate(); err != nil {
				return wireResponse{}, err
			}
			return wireResponse{}, w.browser.Input(r.Context(), input)
		case "credentials":
			record, err := w.browser.Credentials(r.Context())
			return wireResponse{Record: &record}, err
		}
	}
	return wireResponse{}, errors.New("不支持的浏览器操作")
}
