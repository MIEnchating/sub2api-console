package accountworkbench

import (
	"context"
	"errors"
)

func (s *Service) bindPreviewProxy(ctx context.Context, document *privatePreview, result *Preview) error {
	id := document.Settings.PlatformProxyID
	if id == "" && result.Template != nil && result.Template.Config.ProxyID != nil {
		id = *result.Template.Config.ProxyID
	}
	if id == "" {
		return nil
	}
	if document.Settings.ProxyEnabled {
		return errors.New("模板已绑定平台代理，请关闭手动登录代理，统一使用平台代理")
	}
	binding, public, err := s.resolvePlatformProxy(ctx, document.Target, id)
	if err != nil {
		return err
	}
	if result.Template == nil {
		config, err := ExtractConfig(map[string]any{"platform": "openai", "type": "oauth"})
		if err != nil {
			return err
		}
		template := manualTemplate(config)
		template.Name = "默认配置"
		result.Template = &template
	}
	result.Template.Config.ProxyID = &id
	result.Template.Summary.ProxyName = public.Name
	result.Proxy = &public
	document.ProxyBinding = binding
	document.Settings.PlatformProxyID = id
	document.Settings.ProxyURL = binding.URL
	return nil
}
