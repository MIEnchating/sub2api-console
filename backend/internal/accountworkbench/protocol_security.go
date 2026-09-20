package accountworkbench

import (
	"context"
	"errors"
	"net/http"
	"regexp"

	"github.com/MIEnchating/sub2api-console/backend/internal/protocolsdk"
)

var sdkURLPattern = regexp.MustCompile(`https://sentinel\.openai\.com/sentinel/[A-Za-z0-9_-]+/sdk\.js`)

func (p *protocolClient) securityHeaders(ctx context.Context, flow, device string) (http.Header, error) {
	loader, err := p.request(ctx, http.MethodGet, "https://sentinel.openai.com/backend-api/sentinel/sdk.js", nil, nil)
	if err != nil {
		return nil, err
	}
	sdkURL := sdkURLPattern.FindString(string(loader.Body))
	if sdkURL == "" {
		return nil, errors.New("官方安全 SDK 地址无法识别，已停止授权")
	}
	sdk, err := p.request(ctx, http.MethodGet, sdkURL, nil, nil)
	if err != nil {
		return nil, err
	}
	page := protocolAuth + "/log-in/password"
	if flow == "email_otp_validate" {
		page = protocolAuth + "/email-verification"
	}
	tokens, err := protocolsdk.Run(ctx, protocolsdk.Options{SDK: string(sdk.Body), SDKURL: sdkURL, PageURL: page, DeviceID: device, Flow: flow, Request: func(ctx context.Context, request protocolsdk.Request) (protocolsdk.Response, error) {
		if request.URL != "https://sentinel.openai.com/backend-api/sentinel/req" || request.Method != http.MethodPost {
			return protocolsdk.Response{}, errors.New("安全 SDK 请求超出允许范围")
		}
		response, err := p.request(ctx, request.Method, request.URL, []byte(request.Body), http.Header{"Content-Type": {"text/plain;charset=UTF-8"}, "Origin": {"https://sentinel.openai.com"}, "Referer": {"https://sentinel.openai.com/backend-api/sentinel/frame.html"}})
		return protocolsdk.Response{Status: 200, Body: string(response.Body)}, err
	}})
	if err != nil {
		return nil, err
	}
	headers := http.Header{"Openai-Sentinel-Token": {tokens.Token}}
	if tokens.Observer != "" {
		headers.Set("Openai-Sentinel-So-Token", tokens.Observer)
	}
	return headers, nil
}
