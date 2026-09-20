// Package protocolsdk runs the official authorization SDK in an isolated Go
// JavaScript interpreter. It exposes no filesystem, process, or general network API.
package protocolsdk

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/google/uuid"
)

//go:embed environment.js
var environment string

type Request struct {
	URL    string
	Method string
	Body   string
}
type Response struct {
	Status int
	Body   string
}
type Options struct {
	SDK      string
	SDKURL   string
	PageURL  string
	DeviceID string
	Flow     string
	Request  func(context.Context, Request) (Response, error)
}
type Tokens struct {
	Token    string
	Observer string
}
type timer struct {
	id       int64
	at       time.Time
	callback goja.Callable
}

func Run(ctx context.Context, options Options) (Tokens, error) {
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	vm := goja.New()
	stop := context.AfterFunc(ctx, func() { vm.Interrupt("SDK execution expired") })
	defer stop()
	fail := func() (Tokens, error) {
		return Tokens{}, errors.New("官方安全 SDK 执行失败，请稍后重新授权")
	}
	var timers []timer
	var nextID int64
	vm.Set("__setTimer", func(call goja.FunctionCall) goja.Value {
		callback, ok := goja.AssertFunction(call.Argument(0))
		if !ok {
			return goja.Undefined()
		}
		delay := call.Argument(1).ToInteger()
		if delay < 0 {
			delay = 0
		}
		if delay > 60000 {
			delay = 60000
		}
		nextID++
		timers = append(timers, timer{id: nextID, at: time.Now().Add(time.Duration(delay) * time.Millisecond), callback: callback})
		return vm.ToValue(nextID)
	})
	vm.Set("__clearTimer", func(id int64) {
		for i := range timers {
			if timers[i].id == id {
				timers = append(timers[:i], timers[i+1:]...)
				break
			}
		}
	})
	vm.Set("__uuid", func() string { return uuid.NewString() })
	vm.Set("__random", func(size int) []int {
		if size < 0 || size > 65536 {
			panic(vm.NewTypeError("invalid random buffer"))
		}
		raw := make([]byte, size)
		if _, err := rand.Read(raw); err != nil {
			panic(vm.NewTypeError("random unavailable"))
		}
		out := make([]int, size)
		for i, b := range raw {
			out[i] = int(b)
		}
		return out
	})
	vm.Set("__btoa", func(value string) string {
		raw := make([]byte, 0, len(value))
		for _, r := range value {
			if r > 255 {
				panic(vm.NewTypeError("invalid base64 input"))
			}
			raw = append(raw, byte(r))
		}
		return base64.StdEncoding.EncodeToString(raw)
	})
	vm.Set("__atob", func(value string) string {
		raw, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			raw, err = base64.RawStdEncoding.DecodeString(value)
		}
		if err != nil {
			panic(vm.NewTypeError("invalid base64"))
		}
		var out strings.Builder
		for _, b := range raw {
			out.WriteRune(rune(b))
		}
		return out.String()
	})
	vm.Set("__url", func(raw, base string) map[string]any {
		u, err := url.Parse(raw)
		if err != nil {
			panic(vm.NewTypeError("invalid URL"))
		}
		if base != "" {
			b, err := url.Parse(base)
			if err != nil {
				panic(vm.NewTypeError("invalid base URL"))
			}
			u = b.ResolveReference(u)
		}
		search := ""
		if u.RawQuery != "" {
			search = "?" + u.RawQuery
		}
		return map[string]any{"href": u.String(), "origin": u.Scheme + "://" + u.Host, "protocol": u.Scheme + ":", "host": u.Host, "hostname": u.Hostname(), "pathname": u.Path, "search": search, "hash": u.Fragment}
	})
	vm.Set("__request", func(call goja.FunctionCall) goja.Value {
		if options.Request == nil {
			panic(vm.NewTypeError("network unavailable"))
		}
		var req Request
		if err := vm.ExportTo(call.Argument(0), &req); err != nil {
			panic(vm.NewTypeError("invalid request"))
		}
		response, err := options.Request(ctx, req)
		if err != nil {
			panic(vm.NewTypeError("SDK request failed"))
		}
		return vm.ToValue(map[string]any{"status": response.Status, "body": response.Body})
	})
	cfg, _ := json.Marshal(map[string]string{"sdkURL": options.SDKURL, "pageURL": options.PageURL, "deviceID": options.DeviceID})
	vm.Set("__configJSON", string(cfg))
	if _, err := vm.RunString(environment); err != nil {
		return fail()
	}
	if _, err := vm.RunScript(options.SDKURL, options.SDK); err != nil {
		return fail()
	}
	vm.Set("__flow", options.Flow)
	if _, err := vm.RunString(`var __done=false,__error=false,__token=null,__observer=null; Promise.resolve(SentinelSDK.token(__flow)).then(function(value){__token=value;return typeof SentinelSDK.sessionObserverToken==='function'?SentinelSDK.sessionObserverToken(__flow):null;}).then(function(value){__observer=value;__done=true;},function(){__error=true;__done=true;});`); err != nil {
		return fail()
	}
	for !vm.Get("__done").ToBoolean() {
		if ctx.Err() != nil {
			return fail()
		}
		if len(timers) == 0 {
			return fail()
		}
		sort.SliceStable(timers, func(i, j int) bool { return timers[i].at.Before(timers[j].at) })
		current := timers[0]
		timers = timers[1:]
		if wait := time.Until(current.at); wait > 0 {
			t := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				t.Stop()
				return fail()
			case <-t.C:
			}
		}
		if _, err := current.callback(goja.Undefined()); err != nil {
			return fail()
		}
	}
	if vm.Get("__error").ToBoolean() {
		return fail()
	}
	token := vm.Get("__token")
	if goja.IsNull(token) || goja.IsUndefined(token) {
		return fail()
	}
	result := Tokens{Token: token.String()}
	if observer := vm.Get("__observer"); !goja.IsNull(observer) && !goja.IsUndefined(observer) {
		result.Observer = observer.String()
	}
	var parsed struct {
		Error     string `json:"e"`
		ID        string `json:"id"`
		Flow      string `json:"flow"`
		Challenge string `json:"c"`
	}
	if len(result.Token) > 64<<10 || json.Unmarshal([]byte(result.Token), &parsed) != nil || parsed.Error != "" || parsed.ID != options.DeviceID || parsed.Flow != options.Flow || parsed.Challenge == "" || len(result.Observer) > 64<<10 {
		return fail()
	}
	return result, nil
}
