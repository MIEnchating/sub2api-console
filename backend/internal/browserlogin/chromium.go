package browserlogin

import (
	"context"
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
	Executable string
	// Resolve is the network-policy boundary. Production always uses publicAddress.
	Resolve func(context.Context, string) (string, error)
	// CertificateSPKI permits an explicitly pinned TLS certificate (isolated deployments).
	CertificateSPKI string
}
type chromiumBrowser struct {
	ctx     context.Context
	cleanup func()
	once    sync.Once
	record  configstore.AuthRecord
	origin  string
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
	proxy, err := startLoginProxy(ctx, map[string]string{
		net.JoinHostPort(u.Hostname(), port): net.JoinHostPort(ip, port),
		"challenges.cloudflare.com:443":      net.JoinHostPort(challengeIP, "443"),
	})
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
	b := &chromiumBrowser{ctx: browserCtx, record: record, origin: u.Scheme + "://" + u.Host, cleanup: func() {
		browserCancel()
		allocCancel()
		cancel()
		proxy.Close()
		<-processDone
		_ = os.RemoveAll(dir)
	}}
	chromedp.ListenTarget(browserCtx, func(event any) {
		if req, ok := event.(*fetch.EventRequestPaused); ok {
			go func() {
				target := chromedp.FromContext(browserCtx)
				if target == nil || target.Target == nil {
					return
				}
				execCtx := cdp.WithExecutor(browserCtx, target.Target)
				if allowedRequest(req.Request.URL, b.origin) {
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
	err = chromedp.Run(boot, chromedp.EmulateViewport(Width, Height), cdpbrowser.SetDownloadBehavior(cdpbrowser.SetDownloadBehaviorBehaviorDeny), fetch.Enable(), chromedp.Navigate(strings.TrimRight(record.BaseURL, "/")+"/login"))
	if err != nil {
		b.Close()
		return nil, errors.New("验证浏览器启动失败，请检查服务器浏览器运行环境")
	}
	return b, nil
}

func allowedRequest(raw, origin string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return u.Scheme == "https" && u.User == nil && (u.Scheme+"://"+u.Host == origin || u.Host == "challenges.cloudflare.com")
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
	if err := v.Validate(); err != nil {
		return err
	}
	switch v.Kind {
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
