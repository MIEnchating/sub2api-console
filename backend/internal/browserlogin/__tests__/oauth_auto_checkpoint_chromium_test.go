package browserlogin_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

func automaticCheckpointOptions() browserlogin.OAuthOptions {
	options := checkpointOptions()
	options.Recovery.AutoCheckpoint = true
	options.Recovery.CheckpointID = strings.Repeat("e", 48)
	return options
}

func automaticCheckpointRef(options browserlogin.OAuthOptions) browserlogin.OAuthCheckpointRef {
	return browserlogin.OAuthCheckpointRef{ID: options.Recovery.CheckpointID, Owner: options.Recovery.Owner, Lease: options.Recovery.Lease}
}

func awaitAutomaticCheckpoint(t *testing.T, ctx context.Context, reader browserlogin.OAuthCheckpointReader, options browserlogin.OAuthOptions, minimumRevision int64) browserlogin.OAuthCheckpoint {
	t.Helper()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		meta, err := reader.ReadOAuthCheckpoint(ctx, automaticCheckpointRef(options))
		if err == nil && meta.Revision >= minimumRevision {
			return meta
		}
		select {
		case <-ctx.Done():
			t.Fatalf("automatic safe page checkpoint was not available: %v", err)
		case <-ticker.C:
		}
	}
}

func checkpointLoginPage(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/oauth/authorize":
		http.Redirect(w, r, "/phone-verification", http.StatusFound)
	case "/phone-verification":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<form method="post" action="/verify"><label>验证码<input name="code" autofocus></label><button type="submit">验证</button></form>`)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func TestIsolatedOAuthAutomaticCheckpointSurvivesWorkerRestartAndRequiresExactRevision(t *testing.T) {
	var pageLoads atomic.Int64
	restored := make(chan bool, 1)
	ctx, factory := isolatedCheckpointFactory(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/authorize":
			http.SetCookie(w, &http.Cookie{Name: "private-auto", Value: "auto-cookie-secret", Path: "/", Secure: true, HttpOnly: true})
			http.Redirect(w, r, "/phone-verification", http.StatusFound)
		case "/phone-verification":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(w, `<label>验证码<input name="code"></label>`)
			if pageLoads.Add(1) == 1 {
				_, _ = fmt.Fprint(w, `<script>localStorage.setItem("private-auto", "local-secret"); sessionStorage.setItem("private-auto", "session-secret");</script>`)
			} else {
				_, _ = fmt.Fprint(w, `<script>fetch("/restored?ok="+(localStorage.getItem("private-auto")==="local-secret"&&sessionStorage.getItem("private-auto")==="session-secret"));</script>`)
			}
		case "/restored":
			cookie, err := r.Cookie("private-auto")
			restored <- err == nil && cookie.Value == "auto-cookie-secret" && r.URL.Query().Get("ok") == "true"
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	remote, stop := startCheckpointWorker(t, factory)
	options := automaticCheckpointOptions()
	if _, err := remote.OpenOAuth(ctx, options); err != nil {
		t.Fatal(err)
	}
	meta := awaitAutomaticCheckpoint(t, ctx, remote, options, 1)
	stop()
	restarted, _ := startCheckpointWorker(t, factory)
	request := restoredCheckpointOptions(options, meta)
	for _, revision := range []int64{0, meta.Revision + 1} {
		request.Revision = revision
		if _, err := restarted.RestoreOAuth(ctx, request); !errors.Is(err, browserlogin.ErrOAuthCheckpoint) {
			t.Fatal("missing or stale snapshot revision was accepted")
		}
	}
	request.Revision = meta.Revision
	result, err := restarted.RestoreOAuth(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Close()
	select {
	case valid := <-restored:
		if !valid {
			t.Fatal("automatic checkpoint lost private cookie or storage")
		}
	case <-ctx.Done():
		t.Fatal("restored automatic checkpoint page did not report state")
	}
	if !meta.ExpiresAt.Equal(options.Recovery.ExpiresAt) {
		t.Fatal("automatic snapshot extended original authorization deadline")
	}
	if _, err := restarted.RestoreOAuth(ctx, request); !errors.Is(err, browserlogin.ErrOAuthCheckpoint) {
		t.Fatal("consumed automatic snapshot was reused")
	}
}

func TestIsolatedOAuthAutomaticCheckpointRevokesBeforeInputAndAdvancesRevisionAfterSafeNavigation(t *testing.T) {
	var factory browserlogin.Chromium
	options := automaticCheckpointOptions()
	verified := make(chan bool, 1)
	ctx, configured := isolatedCheckpointFactory(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/verify" {
			_, err := factory.ReadOAuthCheckpoint(r.Context(), automaticCheckpointRef(options))
			verified <- errors.Is(err, browserlogin.ErrOAuthCheckpoint)
			http.Redirect(w, r, "/phone-verification", http.StatusSeeOther)
			return
		}
		checkpointLoginPage(w, r)
	}))
	factory = configured
	remote, _ := startCheckpointWorker(t, factory)
	browser, err := remote.OpenOAuth(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()
	first := awaitAutomaticCheckpoint(t, ctx, remote, options, 1)
	// A safe document can be checkpointed before autofocus has been painted.
	if _, err := browser.Screenshot(ctx); err != nil {
		t.Fatal(err)
	}
	if err := browser.Input(ctx, browserlogin.Input{Kind: "click", X: 100, Y: 18}); err != nil {
		t.Fatal(err)
	}
	if err := browser.Input(ctx, browserlogin.Input{Kind: "text", Text: "123456"}); err != nil {
		t.Fatal(err)
	}
	if _, err := remote.ReadOAuthCheckpoint(ctx, automaticCheckpointRef(options)); !errors.Is(err, browserlogin.ErrOAuthCheckpoint) {
		t.Fatal("input left its earlier automatic checkpoint usable")
	}
	if err := browser.Input(ctx, browserlogin.Input{Kind: "key", Key: "Enter"}); err != nil {
		t.Fatal(err)
	}
	select {
	case missing := <-verified:
		if !missing {
			t.Fatal("verification request reached upstream before durable snapshot revocation")
		}
	case <-ctx.Done():
		t.Fatal("manual verification request did not arrive")
	}
	second := awaitAutomaticCheckpoint(t, ctx, remote, options, first.Revision+1)
	if second.ID != first.ID || second.Stage != "sms_code" {
		t.Fatal("next safe stage changed the checkpoint identity")
	}
}

func TestIsolatedOAuthAutomaticCheckpointRevocationFailureBlocksManualInput(t *testing.T) {
	ctx, factory := isolatedCheckpointFactory(t, http.HandlerFunc(checkpointLoginPage))
	remote, _ := startCheckpointWorker(t, factory)
	options := automaticCheckpointOptions()
	browser, err := remote.OpenOAuth(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()
	meta := awaitAutomaticCheckpoint(t, ctx, remote, options, 1)
	if err := os.Chmod(factory.CheckpointDirectory, 0755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(factory.CheckpointDirectory, 0700) })
	if err := browser.Input(ctx, browserlogin.Input{Kind: "text", Text: "123456"}); !errors.Is(err, browserlogin.ErrOAuthCheckpoint) {
		t.Fatal("manual input continued despite failed durable snapshot revocation")
	}
	if _, err := os.Stat(filepath.Join(factory.CheckpointDirectory, meta.ID+".json")); err != nil {
		t.Fatal("failed invalidation unexpectedly removed the saved private state")
	}
	if err := os.Chmod(factory.CheckpointDirectory, 0700); err != nil {
		t.Fatal(err)
	}
}

func TestIsolatedOAuthAutomaticCheckpointRestoreReplacesOnlyMatchingLiveSession(t *testing.T) {
	ctx, factory := isolatedCheckpointFactory(t, http.HandlerFunc(checkpointLoginPage))
	remote, _ := startCheckpointWorker(t, factory)
	options := automaticCheckpointOptions()
	original, err := remote.OpenOAuth(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	defer original.Close()
	meta := awaitAutomaticCheckpoint(t, ctx, remote, options, 1)
	request := restoredCheckpointOptions(options, meta)
	request.Revision = meta.Revision + 1
	if _, err := remote.RestoreOAuth(ctx, request); !errors.Is(err, browserlogin.ErrOAuthCheckpoint) {
		t.Fatal("stale revision replaced live browser")
	}
	if _, err := original.Screenshot(ctx); err != nil {
		t.Fatal("rejected restore closed the original browser")
	}
	request.Revision = meta.Revision
	recovered, err := remote.RestoreOAuth(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	if _, err := original.Screenshot(ctx); !errors.Is(err, browserlogin.ErrSession) {
		t.Fatal("original browser remained accessible alongside restored browser")
	}
	if _, err := recovered.Screenshot(ctx); err != nil {
		t.Fatal("restored browser is unavailable")
	}
}

func TestIsolatedOAuthAutomaticCheckpointDetachPreservesAndExplicitCloseRevokes(t *testing.T) {
	ctx, factory := isolatedCheckpointFactory(t, http.HandlerFunc(checkpointLoginPage))
	remote, _ := startCheckpointWorker(t, factory)
	options := automaticCheckpointOptions()
	browser, err := remote.OpenOAuth(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	meta := awaitAutomaticCheckpoint(t, ctx, remote, options, 1)
	browser.(browserlogin.OAuthCheckpointPreserver).ClosePreservingOAuthCheckpoint()
	if _, err := remote.ReadOAuthCheckpoint(ctx, automaticCheckpointRef(options)); err != nil {
		t.Fatal("server detachment deleted an authorized safe checkpoint")
	}
	request := restoredCheckpointOptions(options, meta)
	request.Revision = meta.Revision
	recovered, err := remote.RestoreOAuth(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	awaitAutomaticCheckpoint(t, ctx, remote, request.Options, meta.Revision+1)
	recovered.Close()
	if _, err := remote.ReadOAuthCheckpoint(ctx, automaticCheckpointRef(request.Options)); !errors.Is(err, browserlogin.ErrOAuthCheckpoint) {
		t.Fatal("explicit close retained a usable checkpoint")
	}
}

func TestIsolatedOAuthAutomaticCheckpointRevokesBeforePageWritesAndCallback(t *testing.T) {
	for _, operation := range []string{"write", "callback", "blocked-write"} {
		t.Run(operation, func(t *testing.T) {
			var trigger atomic.Bool
			var writes atomic.Int64
			completed := make(chan bool, 1)
			var factory browserlogin.Chromium
			options := automaticCheckpointOptions()
			ctx, configured := isolatedCheckpointFactory(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/phone-verification":
					w.Header().Set("Content-Type", "text/html; charset=utf-8")
					_, _ = fmt.Fprintf(w, `<label>验证码<input name="code"></label><script>
					const poll=setInterval(async()=>{const go=await (await fetch('/trigger')).text();if(go!=='go')return;clearInterval(poll);
					if(%q==='callback'){location.href=%q;return;}
					fetch('/write',{method:'POST'}).then(()=>fetch('/complete?blocked=false')).catch(()=>fetch('/complete?blocked=true'));
					},200);</script>`, operation, options.RedirectURI+"?state="+options.State+"&code=isolated-auto-code")
				case "/trigger":
					if trigger.Load() {
						_, _ = fmt.Fprint(w, "go")
					} else {
						_, _ = fmt.Fprint(w, "wait")
					}
				case "/write":
					writes.Add(1)
					_, err := factory.ReadOAuthCheckpoint(r.Context(), automaticCheckpointRef(options))
					completed <- errors.Is(err, browserlogin.ErrOAuthCheckpoint)
					w.WriteHeader(http.StatusNoContent)
				case "/complete":
					if operation == "blocked-write" {
						completed <- r.URL.Query().Get("blocked") == "true"
					}
					w.WriteHeader(http.StatusNoContent)
				default:
					checkpointLoginPage(w, r)
				}
			}))
			factory = configured
			remote, _ := startCheckpointWorker(t, factory)
			browser, err := remote.OpenOAuth(ctx, options)
			if err != nil {
				t.Fatal(err)
			}
			defer browser.Close()
			awaitAutomaticCheckpoint(t, ctx, remote, options, 1)
			if operation == "blocked-write" {
				if err := os.Chmod(factory.CheckpointDirectory, 0755); err != nil {
					t.Fatal(err)
				}
				defer func() { _ = os.Chmod(factory.CheckpointDirectory, 0700) }()
			}
			trigger.Store(true)
			if operation == "callback" {
				ticker := time.NewTicker(20 * time.Millisecond)
				defer ticker.Stop()
				for {
					_, err := browser.AuthorizationCode(ctx)
					if err == nil {
						break
					}
					if !errors.Is(err, browserlogin.ErrOAuthPending) {
						t.Fatal(err)
					}
					select {
					case <-ctx.Done():
						t.Fatal("official callback did not arrive")
					case <-ticker.C:
					}
				}
				if _, err := remote.ReadOAuthCheckpoint(ctx, automaticCheckpointRef(options)); !errors.Is(err, browserlogin.ErrOAuthCheckpoint) {
					t.Fatal("authorization callback left a reusable snapshot")
				}
				return
			}
			select {
			case valid := <-completed:
				if !valid {
					t.Fatal("page write proceeded with reusable or unrevokable private snapshot")
				}
			case <-ctx.Done():
				t.Fatal("page request did not report its result")
			}
			if operation == "blocked-write" && writes.Load() != 0 {
				t.Fatal("failed durable revocation allowed a page write to reach upstream")
			}
		})
	}
}

func TestIsolatedOAuthAutomaticCheckpointRequiresExplicitConsent(t *testing.T) {
	ctx, factory := isolatedCheckpointFactory(t, http.HandlerFunc(checkpointLoginPage))
	remote, _ := startCheckpointWorker(t, factory)
	options := checkpointOptions()
	browser, err := remote.OpenOAuth(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()
	if _, err := browser.Screenshot(ctx); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(factory.CheckpointDirectory)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".json") {
			t.Fatal("automatic checkpoint was saved without explicit consent")
		}
	}
}
