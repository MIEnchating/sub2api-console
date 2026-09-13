package modelcheck

import (
	"encoding/xml"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const svgXMLNamespace = "http://www.w3.org/XML/1998/namespace"
const svgXLinkNamespace = "http://www.w3.org/1999/xlink"

// Keep CSS restricted to static SVG presentation. No selectors, custom properties,
// animations, imports, or arbitrary resource-bearing CSS properties are accepted.
var animationStyleProperties = wordSet("fill fill-opacity fill-rule clip-rule stroke stroke-width stroke-linecap stroke-linejoin stroke-miterlimit stroke-dasharray stroke-dashoffset stroke-opacity opacity color transform transform-origin transform-box font-size font-family font-weight font-style text-anchor dominant-baseline alignment-baseline letter-spacing word-spacing text-decoration stop-color stop-opacity clip-path mask vector-effect paint-order shape-rendering text-rendering color-interpolation display visibility overflow")
var animationCSSFunctions = wordSet("rgb rgba hsl hsla matrix translate translatex translatey scale scalex scaley rotate skew skewx skewy url calc min max clamp")
var animationCSSFunction = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9-]*)\s*\(`)
var animationURLFunction = regexp.MustCompile(`(?i)url\s*\(`)

func animationAttributeError(element, name string) error {
	// Values can contain upstream content: report only bounded XML names.
	if len(name) > 64 {
		name = name[:64] + "…"
	}
	return fmt.Errorf("生成的 SVG 在 %s 元素中包含不支持或不安全的属性“%s”，请重试", element, name)
}

func sanitizeAnimationAttributes(element xml.StartElement) ([]xml.Attr, error) {
	attrs := make([]xml.Attr, 0, len(element.Attr)+1)
	seenNames := map[xml.Name]bool{}
	seenValues := map[string]string{}
	for _, attr := range element.Attr {
		name := attr.Name.Local
		if seenNames[attr.Name] {
			return nil, animationAttributeError(element.Name.Local, name)
		}
		seenNames[attr.Name] = true
		if attr.Name.Space == "xmlns" || name == "xmlns" {
			continue
		}
		if attr.Name.Space == svgXMLNamespace && name == "space" {
			if attr.Value != "default" && attr.Value != "preserve" {
				return nil, animationAttributeError(element.Name.Local, "xml:space")
			}
			attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "xml:space"}, Value: attr.Value})
			continue
		}
		if attr.Name.Space != "" && !(attr.Name.Space == svgXLinkNamespace && name == "href") {
			return nil, animationAttributeError(element.Name.Local, name)
		}
		if name == "style" {
			style, err := sanitizeAnimationStyle(attr.Value)
			if err != nil {
				return nil, err
			}
			if style != "" {
				attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "style"}, Value: style})
			}
			continue
		}
		if !animationAttributes[name] {
			return nil, animationAttributeError(element.Name.Local, name)
		}
		if previous, exists := seenValues[name]; exists {
			// SVG 1.1/2 compatibility may supply identical href and xlink:href.
			if name == "href" && previous == attr.Value {
				continue
			}
			return nil, animationAttributeError(element.Name.Local, name)
		}
		seenValues[name] = attr.Value
		if err := validateAnimationAttributeValue(name, attr.Value); err != nil {
			return nil, err
		}
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: name}, Value: attr.Value})
	}
	return attrs, nil
}

func validateAnimationAttributeValue(name, value string) error {
	if name == "values" {
		for _, item := range strings.Split(value, ";") {
			if err := validateAnimationAttributeValue("", item); err != nil {
				return err
			}
		}
		return nil
	}
	lower := strings.ToLower(strings.TrimSpace(value))
	if animationURLFunction.MatchString(value) && !localPaint.MatchString(strings.TrimSpace(value)) {
		return errors.New("动画不能引用外部资源")
	}
	if strings.ContainsAny(value, "\\<>") || strings.Contains(lower, "javascript:") || strings.Contains(lower, "data:") {
		return errors.New("动画包含不安全的属性值")
	}
	if name == "href" && (!strings.HasPrefix(value, "#") || len(value) < 2) {
		return errors.New("动画不能引用外部资源")
	}
	if name == "attributeName" && !animationAnimatedAttributes[value] {
		return errors.New("动画尝试修改不允许的属性")
	}
	return nil
}

func sanitizeAnimationStyle(style string) (string, error) {
	if strings.ContainsAny(style, "\\{}@<>") || strings.Contains(style, "/*") || strings.Contains(style, "*/") {
		return "", errors.New("动画的 style 属性包含不支持或不安全的样式语法")
	}
	declarations := strings.Split(style, ";")
	if len(declarations) > 64 {
		return "", errors.New("动画的 style 属性过于复杂")
	}
	safe := make([]string, 0, len(declarations))
	for _, declaration := range declarations {
		declaration = strings.TrimSpace(declaration)
		if declaration == "" {
			continue
		}
		property, value, found := strings.Cut(declaration, ":")
		property = strings.ToLower(strings.TrimSpace(property))
		if !found {
			return "", errors.New("动画的 style 属性不是有效的静态样式声明")
		}
		if !animationStyleProperties[property] {
			return "", animationAttributeError("style", property)
		}
		value = strings.TrimSpace(value)
		important := ""
		if strings.HasSuffix(strings.ToLower(value), "!important") {
			value = strings.TrimSpace(value[:len(value)-len("!important")])
			important = " !important"
		}
		if value == "" || strings.Contains(value, "!") {
			return "", animationAttributeError("style", property)
		}
		if err := validateAnimationAttributeValue(property, value); err != nil {
			return "", err
		}
		for _, function := range animationCSSFunction.FindAllStringSubmatch(value, -1) {
			if !animationCSSFunctions[strings.ToLower(function[1])] {
				return "", animationAttributeError("style", property)
			}
		}
		safe = append(safe, property+":"+value+important)
	}
	return strings.Join(safe, ";"), nil
}
