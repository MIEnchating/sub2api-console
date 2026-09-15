package browserlogin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	cdpbrowser "github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/storage"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

type Chromium struct {
	Executable          string
	CheckpointDirectory string
	// Resolve is the network-policy boundary. Production always uses publicAddress.
	Resolve func(context.Context, string) (string, error)
	// CertificateSPKI permits an explicitly pinned TLS certificate (isolated deployments).
	CertificateSPKI string
}
type chromiumBrowser struct {
	ctx            context.Context
	cleanup        func()
	once           sync.Once
	record         configstore.AuthRecord
	origin         string
	allowedOrigins []string
	callback       *oauthCallback
	securityAuth   bool
	security       atomic.Pointer[securityChromium]
	challenge      atomic.Pointer[string]
	recovery       *oauthRecoveryState
	seed           *oauthCheckpointDocument
}

type chromiumSessionState struct {
	recovery *oauthRecoveryState
	security *securityChromium
	seed     *oauthCheckpointDocument
}

// Resolve before launch and pin Chromium DNS so the login page cannot redirect
// this server-side browser into private services or change DNS after validation.
func publicAddress(ctx context.Context, host string) (string, error) {
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(ips) == 0 {
		return "", errors.New("上游域名解析失败")
	}
	for _, item := range ips {
		if !item.IP.IsGlobalUnicast() || item.IP.IsPrivate() || item.IP.IsLoopback() || item.IP.IsLinkLocalUnicast() {
			return "", errors.New("浏览器验证只允许公开上游地址")
		}
	}
	return ips[0].IP.String(), nil
}

func (f Chromium) Open(ctx context.Context, record configstore.AuthRecord) (Browser, error) {
	return f.openChromium(ctx, record, strings.TrimRight(record.BaseURL, "/")+"/login", []string{strings.TrimRight(record.BaseURL, "/")}, nil, "")
}

func (f Chromium) OpenOAuth(ctx context.Context, options OAuthOptions) (OAuthBrowser, error) {
	if err := options.Validate(); err != nil {
		return nil, err
	}
	if options.Recovery != nil && options.Recovery.AutoCheckpoint {
		dir, err := openOAuthCheckpointDirectory(f.CheckpointDirectory)
		if err != nil {
			return nil, err
		}
		dir.close()
	}
	b, err := f.openChromium(ctx, configstore.AuthRecord{BaseURL: "https://auth.openai.com", Host: "auth.openai.com"}, options.AuthorizationURL, oauthAllowedOrigins(), &oauthCallback{state: options.State, redirectURI: options.RedirectURI}, options.ProxyURL, chromiumSessionState{recovery: newOAuthRecoveryState(f, options)})
	if err != nil {
		return nil, err
	}
	browser := b.(*chromiumBrowser)
	browser.startAutomaticCheckpoints()
	return browser, nil
}

type oauthCallback struct {
	mu                       sync.Mutex
	state, redirectURI, code string
	frameID                  cdp.FrameID
	err                      error
}

func (c *oauthCallback) capture(raw string) {
	u, err := url.Parse(raw)
	if err != nil || !isOAuthCallbackURL(raw, c.redirectURI) {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.code != "" || c.err != nil {
		return
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(query["state"]) != 1 || query.Get("state") != c.state {
		c.err = ErrOAuthState
		return
	}
	if _, rejected := query["error"]; rejected {
		c.err = ErrOAuthRejected
		return
	}
	code := query.Get("code")
	if len(query["code"]) != 1 || code == "" || len(code) > 8192 || strings.ContainsFunc(code, func(r rune) bool { return r <= ' ' || r == 127 }) {
		c.err = ErrOAuthRejected
		return
	}
	c.code = code
}

func (f Chromium) openChromium(ctx context.Context, record configstore.AuthRecord, navigateURL string, allowedOrigins []string, callback *oauthCallback, proxyURL string, initial ...chromiumSessionState) (Browser, error) {
	u, err := url.Parse(record.BaseURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || (u.Port() != "" && u.Port() != "443" && f.Resolve == nil) {
		return nil, errors.New("浏览器验证需要使用 HTTPS 默认端口的上游地址")
	}
	executable := f.Executable
	if executable == "" {
		executable = "chromium-browser"
	}
	executable, err = exec.LookPath(executable)
	if err != nil {
		return nil, errors.New("服务器未安装验证浏览器，请使用包含 Chromium 和 Xvfb 的控制台镜像")
	}
	resolveCtx, resolveCancel := context.WithTimeout(ctx, 10*time.Second)
	defer resolveCancel()
	resolver := f.Resolve
	if resolver == nil {
		resolver = publicAddress
	}
	ip, err := resolver(resolveCtx, u.Hostname())
	if err != nil {
		return nil, err
	}
	challengeIP, err := resolver(resolveCtx, "challenges.cloudflare.com")
	if err != nil {
		return nil, err
	}
	port := u.Port()
	if port == "" {
		port = "443"
	}
	destinations := map[string]string{
		net.JoinHostPort(u.Hostname(), port): net.JoinHostPort(ip, port),
		"challenges.cloudflare.com:443":      net.JoinHostPort(challengeIP, "443"),
	}
	for _, origin := range allowedOrigins {
		parsed, parseErr := url.Parse(origin)
		if parseErr != nil || parsed.Scheme != "https" || parsed.Hostname() == "" {
			return nil, errors.New("OAuth 授权来源无效")
		}
		if parsed.Hostname() == u.Hostname() {
			continue
		}
		otherIP, resolveErr := resolver(resolveCtx, parsed.Hostname())
		if resolveErr != nil {
			return nil, resolveErr
		}
		otherPort := parsed.Port()
		if otherPort == "" {
			otherPort = "443"
		}
		destinations[net.JoinHostPort(parsed.Hostname(), otherPort)] = net.JoinHostPort(otherIP, otherPort)
	}
	for _, challenge := range []string{"static.cloudflareinsights.com", "challenges.cloudflare.com"} {
		if callback == nil {
			break
		}
		if _, ok := destinations[net.JoinHostPort(challenge, "443")]; ok {
			continue
		}
		challengeIP, resolveErr := resolver(resolveCtx, challenge)
		if resolveErr != nil {
			return nil, resolveErr
		}
		destinations[net.JoinHostPort(challenge, "443")] = net.JoinHostPort(challengeIP, "443")
	}
	proxy, err := startLoginProxy(ctx, destinations, proxyURL, f.Resolve)
	if err != nil {
		return nil, errors.New("浏览器网络隔离服务启动失败")
	}
	dir, err := os.MkdirTemp("", "console-browser-login-")
	if err != nil {
		proxy.Close()
		return nil, err
	}
	processCtx, cancel := context.WithCancel(ctx)
	cleanup := func() { cancel(); proxy.Close(); _ = os.RemoveAll(dir) }
	// Xvfb chooses a free display and reports it through a private pipe.
	read, write, err := os.Pipe()
	if err != nil {
		cleanup()
		return nil, err
	}
	xvfb := exec.CommandContext(processCtx, "Xvfb", "-displayfd", "3", "-screen", "0", "1100x900x24", "-nolisten", "tcp")
	xvfb.ExtraFiles = []*os.File{write}
	if err = xvfb.Start(); err != nil {
		read.Close()
		write.Close()
		cleanup()
		return nil, errors.New("无法启动浏览器显示服务，请检查 Xvfb 安装")
	}
	write.Close()
	processDone := make(chan struct{})
	go func() { _ = xvfb.Wait(); close(processDone) }()
	displayResult := make(chan string, 1)
	go func() {
		defer read.Close()
		var n int
		_, err := fmt.Fscan(read, &n)
		if err != nil {
			displayResult <- ""
		} else {
			displayResult <- strconv.Itoa(n)
		}
	}()
	var display string
	select {
	case display = <-displayResult:
	case <-ctx.Done():
	case <-time.After(10 * time.Second):
	}
	if display == "" {
		read.Close()
		cleanup()
		<-processDone
		return nil, errors.New("浏览器显示服务启动超时")
	}
	rules := "MAP * ~NOTFOUND, EXCLUDE 127.0.0.1"
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, chromedp.ExecPath(executable), chromedp.UserDataDir(dir), chromedp.Flag("headless", false), chromedp.Flag("no-sandbox", true), chromedp.Flag("disable-dev-shm-usage", true), chromedp.Flag("host-resolver-rules", rules), chromedp.Flag("disable-quic", true), chromedp.Flag("proxy-server", proxy.Address()), chromedp.Flag("proxy-bypass-list", "<-loopback>"), chromedp.Flag("force-webrtc-ip-handling-policy", "disable_non_proxied_udp"), chromedp.Env("DISPLAY=:"+display, "XDG_CONFIG_HOME="+dir, "XDG_CACHE_HOME="+dir), chromedp.WindowSize(Width, 900))
	if f.CertificateSPKI != "" {
		opts = append(opts, chromedp.Flag("ignore-certificate-errors-spki-list", f.CertificateSPKI))
	}
	alloc, allocCancel := chromedp.NewExecAllocator(processCtx, opts...)
	browserCtx, browserCancel := chromedp.NewContext(alloc)
	b := &chromiumBrowser{ctx: browserCtx, record: record, origin: u.Scheme + "://" + u.Host, allowedOrigins: append([]string(nil), allowedOrigins...), callback: callback, cleanup: func() {
		browserCancel()
		allocCancel()
		cancel()
		proxy.Close()
		<-processDone
		_ = os.RemoveAll(dir)
	}}
	if len(initial) == 1 {
		b.recovery, b.seed = initial[0].recovery, initial[0].seed
		b.securityAuth = initial[0].security != nil
		b.security.Store(initial[0].security)
	}
	chromedp.ListenTarget(browserCtx, func(event any) {
		b.observeChallenge(event)
		b.observeOAuthRecovery(event)
		if req, ok := event.(*fetch.EventRequestPaused); ok {
			go func() {
				target := chromedp.FromContext(browserCtx)
				if target == nil || target.Target == nil {
					return
				}
				execCtx := cdp.WithExecutor(browserCtx, target.Target)
				if !b.allowRecoveryRequest(req) {
					_ = fetch.FailRequest(req.RequestID, network.ErrorReasonBlockedByClient).Do(execCtx)
					return
				}
				if security := b.security.Load(); security != nil {
					if !security.allowSecurityRequest(req) {
						_ = fetch.FailRequest(req.RequestID, network.ErrorReasonBlockedByClient).Do(execCtx)
						return
					}
					if security.capturePasswordResponse(execCtx, req) {
						return
					}
				}
				if b.seed != nil && req.ResourceType == network.ResourceTypeDocument && req.Request.URL == securityCheckpointBootstrapURL {
					_ = fetch.FulfillRequest(req.RequestID, 200).WithResponseHeaders([]*fetch.HeaderEntry{{Name: "Content-Type", Value: "text/html"}, {Name: "Cache-Control", Value: "no-store"}, {Name: "Content-Security-Policy", Value: "default-src 'none'; frame-ancestors 'none'"}}).WithBody(base64.StdEncoding.EncodeToString([]byte("<!doctype html><html><body></body></html>"))).Do(execCtx)
					return
				}
				if b.callback != nil && isOAuthCallbackURL(req.Request.URL, b.callback.redirectURI) {
					b.callback.mu.Lock()
					mainDocument := req.ResourceType == network.ResourceTypeDocument && req.FrameID == b.callback.frameID
					b.callback.mu.Unlock()
					if !mainDocument {
						_ = fetch.FailRequest(req.RequestID, network.ErrorReasonBlockedByClient).Do(execCtx)
						return
					}
					b.callback.capture(req.Request.URL)
					_ = fetch.FulfillRequest(req.RequestID, 200).WithResponseHeaders([]*fetch.HeaderEntry{
						{Name: "Content-Type", Value: "text/html; charset=utf-8"}, {Name: "Cache-Control", Value: "no-store"},
						{Name: "Content-Security-Policy", Value: "default-src 'none'; frame-ancestors 'none'; base-uri 'none'"}, {Name: "Referrer-Policy", Value: "no-referrer"},
					}).WithBody(base64.StdEncoding.EncodeToString([]byte("<html><body>授权回调已收到，可以关闭此页面。</body></html>"))).Do(execCtx)
				} else if allowedRequest(req.Request.URL, b.origin, b.allowedOrigins) {
					_ = fetch.ContinueRequest(req.RequestID).Do(execCtx)
				} else {
					_ = fetch.FailRequest(req.RequestID, network.ErrorReasonBlockedByClient).Do(execCtx)
				}
			}()
		}
	})
	boot, bootCancel := context.WithTimeout(browserCtx, 30*time.Second)
	defer bootCancel()
	// Allocate the root browser with its lifetime context. Cancelling a short
	// startup child on first allocation would otherwise kill the browser.
	startupTimer := time.AfterFunc(30*time.Second, browserCancel)
	err = chromedp.Run(browserCtx)
	startupTimer.Stop()
	if err != nil {
		b.Close()
		return nil, errors.New("验证浏览器进程启动失败")
	}
	if callback != nil {
		if err = chromedp.Run(boot, chromedp.ActionFunc(func(c context.Context) error {
			tree, frameErr := page.GetFrameTree().Do(c)
			if frameErr != nil {
				return frameErr
			}
			callback.mu.Lock()
			callback.frameID = tree.Frame.ID
			callback.mu.Unlock()
			return nil
		})); err != nil {
			b.Close()
			return nil, errors.New("无法隔离 OAuth 授权回调")
		}
	}
	var seedScript page.ScriptIdentifier
	err = chromedp.Run(boot, chromedp.EmulateViewport(Width, Height), cdpbrowser.SetDownloadBehavior(cdpbrowser.SetDownloadBehaviorBehaviorDeny), network.Enable(), fetch.Enable(), chromedp.ActionFunc(func(c context.Context) error {
		var seedErr error
		seedScript, seedErr = b.seedOAuthCheckpoint(c)
		return seedErr
	}), chromedp.Navigate(navigateURL), chromedp.ActionFunc(func(c context.Context) error {
		if seedScript != "" {
			return page.RemoveScriptToEvaluateOnNewDocument(seedScript).Do(c)
		}
		return nil
	}))
	if err != nil {
		b.Close()
		return nil, errors.New("验证浏览器启动失败，请检查服务器浏览器运行环境")
	}
	return b, nil
}

func isOAuthCallbackURL(raw, redirect string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	r, err := url.Parse(redirect)
	return err == nil && u.User == nil && u.Fragment == "" && u.Scheme == r.Scheme && u.Host == r.Host && u.EscapedPath() == r.EscapedPath()
}

func allowedRequest(raw, origin string, extra ...[]string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if u.Scheme != "https" || u.User != nil {
		return false
	}
	if u.Scheme+"://"+u.Host == origin || u.Host == "challenges.cloudflare.com" || u.Host == "static.cloudflareinsights.com" {
		return true
	}
	for _, origins := range extra {
		for _, allowed := range origins {
			if u.Scheme+"://"+u.Host == allowed {
				return true
			}
		}
	}
	return false
}
func (b *chromiumBrowser) run(ctx context.Context, actions ...chromedp.Action) error {
	run, cancel := context.WithTimeout(b.ctx, 15*time.Second)
	defer cancel()
	stop := context.AfterFunc(ctx, cancel)
	defer stop()
	return chromedp.Run(run, actions...)
}
func (b *chromiumBrowser) Screenshot(ctx context.Context) ([]byte, error) {
	var result []byte
	err := b.run(ctx, chromedp.ActionFunc(func(c context.Context) error {
		var err error
		result, err = page.CaptureScreenshot().WithFormat(page.CaptureScreenshotFormatJpeg).WithQuality(70).Do(c)
		return err
	}))
	return result, err
}
func (b *chromiumBrowser) Input(ctx context.Context, v Input) error {
	if b.recovery != nil {
		b.recovery.op.Lock()
		defer b.recovery.op.Unlock()
	}
	if err := v.Validate(); err != nil {
		return err
	}
	if err := b.checkRecoveryInput(true); err != nil {
		return err
	}
	switch v.Kind {
	case "reload":
		if b.callback != nil || b.securityAuth {
			return errors.New("当前授权会话不支持重新加载登录页")
		}
		// Navigate with GET to avoid replaying a submitted login form.
		return b.run(ctx, chromedp.Navigate(strings.TrimRight(b.record.BaseURL, "/")+"/login"))
	case "click":
		return b.run(ctx, chromedp.MouseClickXY(v.X, v.Y))
	case "text":
		return b.run(ctx, chromedp.ActionFunc(func(c context.Context) error { return input.InsertText(v.Text).Do(c) }))
	case "scroll":
		return b.run(ctx, chromedp.ActionFunc(func(c context.Context) error {
			return input.DispatchMouseEvent(input.MouseWheel, Width/2, Height/2).WithDeltaY(v.Delta).WithDeltaX(0).Do(c)
		}))
	case "key":
		keys := map[string]string{"Tab": kb.Tab, "Enter": kb.Enter, "Backspace": kb.Backspace, "Delete": kb.Delete, "Escape": kb.Escape, "ArrowLeft": kb.ArrowLeft, "ArrowRight": kb.ArrowRight, "ArrowUp": kb.ArrowUp, "ArrowDown": kb.ArrowDown, "Home": kb.Home, "End": kb.End}
		key := keys[v.Key]
		if v.Shift {
			key = kb.Shift + key
		}
		return b.run(ctx, chromedp.KeyEvent(key))
	}
	return errors.New("浏览器操作无效")
}
func (b *chromiumBrowser) Credentials(ctx context.Context) (configstore.AuthRecord, error) {
	var values struct {
		Origin  string `json:"origin"`
		Access  string `json:"access"`
		Refresh string `json:"refresh"`
		UA      string `json:"ua"`
	}
	var cookies []*network.Cookie
	err := b.run(ctx, chromedp.Evaluate(`({origin:location.origin,access:localStorage.getItem("auth_token")||localStorage.getItem("access_token")||sessionStorage.getItem("auth_token")||sessionStorage.getItem("access_token")||"",refresh:localStorage.getItem("refresh_token")||sessionStorage.getItem("refresh_token")||"",ua:navigator.userAgent})`, &values), chromedp.ActionFunc(func(c context.Context) error { var e error; cookies, e = storage.GetCookies().Do(c); return e }))
	if err != nil {
		return configstore.AuthRecord{}, errors.New("读取登录结果失败，请保持上游登录页面打开")
	}
	if values.Origin != b.origin {
		return configstore.AuthRecord{}, errors.New("请返回上游网站完成登录")
	}
	record := b.record
	record.Cookies = map[string]string{}
	host, _ := url.Parse(b.origin)
	for _, cookie := range cookies {
		domain := strings.TrimPrefix(cookie.Domain, ".")
		if host.Hostname() != domain && !strings.HasSuffix(host.Hostname(), "."+domain) {
			continue
		}
		record.Cookies[cookie.Name] = cookie.Value
		if cookie.Name == "sub2api_refresh_token" {
			values.Refresh = cookie.Value
		}
	}
	// Some frontends serialize strings as JSON rather than storing them directly.
	for _, value := range []*string{&values.Access, &values.Refresh} {
		var decoded string
		if json.Unmarshal([]byte(*value), &decoded) == nil {
			*value = decoded
		}
	}
	if values.Access == "" || values.Refresh == "" {
		return configstore.AuthRecord{}, errors.New("尚未取得完整登录凭据，请先在上游页面完成登录后重试")
	}
	record.AccessToken = &values.Access
	record.RefreshToken = &values.Refresh
	record.AdminKey = nil
	record.UserID = nil
	record.AuthMode = "sub2api_user_token"
	record.Headers = map[string]string{"User-Agent": values.UA, "Origin": b.origin, "Referer": b.origin + "/", "Accept-Language": "zh"}
	return record, nil
}
func (b *chromiumBrowser) Close() { b.once.Do(b.cleanup) }

func (b *chromiumBrowser) AuthorizationCode(ctx context.Context) (OAuthResult, error) {
	if b.callback == nil {
		return OAuthResult{}, errors.New("当前浏览器不是 OAuth 授权会话")
	}
	if err := ctx.Err(); err != nil {
		return OAuthResult{}, err
	}
	if b.ctx.Err() != nil {
		return OAuthResult{}, ErrSession
	}
	b.callback.mu.Lock()
	defer b.callback.mu.Unlock()
	if b.callback.err != nil {
		return OAuthResult{}, b.callback.err
	}
	if b.callback.code == "" {
		return OAuthResult{}, ErrOAuthPending
	}
	return OAuthResult{Code: b.callback.code, State: b.callback.state}, nil
}
