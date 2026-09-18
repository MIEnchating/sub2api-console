package onboarding

import (
	"errors"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// NormalizeModelMapping accepts exact names, matching direct account probes.
func NormalizeModelMapping(requested map[string]string) (map[string]string, error) {
	if len(requested) > 100 {
		return nil, errors.New("最多配置 100 条模型映射")
	}
	mapping := make(map[string]string, len(requested))
	for source, target := range requested {
		source, target = strings.TrimSpace(source), strings.TrimSpace(target)
		for _, model := range []string{source, target} {
			if model == "" || utf8.RuneCountInString(model) > 256 ||
				strings.ContainsAny(model, "*?[]{}^$|\\") ||
				strings.ContainsFunc(model, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
				return nil, errors.New("模型映射须填写 1 到 256 个字符的精确模型名称，不支持空白、通配符或控制字符")
			}
		}
		if _, exists := mapping[source]; exists {
			return nil, errors.New("模型映射的请求模型不能重复")
		}
		mapping[source] = target
	}
	return mapping, nil
}

func mergeOnboardingModelMapping(models []string, overrides map[string]string) (map[string]string, []string) {
	mapping := identityModelMapping(models)
	for source, target := range overrides {
		mapping[source] = target
	}
	available := make([]string, 0, len(mapping))
	for source := range mapping {
		available = append(available, source)
	}
	sort.Strings(available)
	return mapping, available
}
