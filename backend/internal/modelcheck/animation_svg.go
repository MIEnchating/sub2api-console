package modelcheck

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"regexp"
	"strings"
)

var animationElements = wordSet("svg g defs title desc path rect circle ellipse line polyline polygon text tspan linearGradient radialGradient stop clipPath mask pattern use animate animateTransform animateMotion mpath set")
var animationAttributes = wordSet("version baseProfile class role aria-label aria-labelledby aria-describedby aria-hidden focusable stroke-miterlimit vector-effect paint-order shape-rendering text-rendering color color-interpolation clip-rule display visibility overflow font-style text-decoration alignment-baseline letter-spacing word-spacing textLength lengthAdjust id xmlns viewBox width height x y x1 y1 x2 y2 cx cy r rx ry d points fill fill-opacity fill-rule stroke stroke-width stroke-linecap stroke-linejoin stroke-dasharray stroke-dashoffset stroke-opacity opacity transform transform-origin font-size font-family font-weight text-anchor dominant-baseline dx dy offset stop-color stop-opacity gradientUnits gradientTransform spreadMethod clip-path clipPathUnits mask maskUnits maskContentUnits patternUnits patternContentUnits patternTransform preserveAspectRatio href attributeName attributeType type from to by values keyTimes keySplines calcMode dur begin end repeatCount repeatDur fill-mode additive accumulate rotate path keyPoints restart")
var animationAnimatedAttributes = wordSet("transform d points x y cx cy r rx ry x1 y1 x2 y2 dx dy width height opacity fill fill-opacity stroke stroke-width stroke-dashoffset stroke-opacity rotate")
var localPaint = regexp.MustCompile(`(?i)^url\(\s*(?:#[a-zA-Z_][a-zA-Z0-9_.:-]*|"#[a-zA-Z_][a-zA-Z0-9_.:-]*"|'#[a-zA-Z_][a-zA-Z0-9_.:-]*')\s*\)$`)

func wordSet(words string) map[string]bool {
	result := map[string]bool{}
	for _, word := range strings.Fields(words) {
		result[word] = true
	}
	return result
}

func sanitizeAnimationSVG(text string) (string, error) {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```") {
		first := strings.IndexByte(text, '\n')
		if first >= 0 && strings.HasSuffix(text, "```") {
			text = strings.TrimSpace(text[first+1 : len(text)-3])
		}
	}
	if len(text) > 24<<10 {
		return "", errors.New("生成的 SVG 超过 24 KB，请更换模型后重试")
	}
	decoder := xml.NewDecoder(strings.NewReader(text))
	var output bytes.Buffer
	encoder := xml.NewEncoder(&output)
	depth, elements, roots := 0, 0, 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", errors.New("生成内容不是完整 SVG，请重试")
		}
		switch value := token.(type) {
		case xml.StartElement:
			if !animationElements[value.Name.Local] || (value.Name.Space != "" && value.Name.Space != "http://www.w3.org/2000/svg") {
				return "", errors.New("生成的 SVG 包含不支持或不安全的元素，请重试")
			}
			if depth == 0 {
				roots++
				if roots != 1 || value.Name.Local != "svg" {
					return "", errors.New("生成内容必须是单个 SVG 动画")
				}
			}
			elements++
			depth++
			if elements > 2000 || depth > 64 {
				return "", errors.New("生成的 SVG 结构过于复杂，请重试")
			}
			attrs, err := sanitizeAnimationAttributes(value)
			if err != nil {
				return "", err
			}
			if depth == 1 {
				attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "xmlns"}, Value: "http://www.w3.org/2000/svg"})
			}
			value.Name.Space = ""
			value.Attr = attrs
			token = value
		case xml.EndElement:
			depth--
			value.Name.Space = ""
			token = value
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(value)) != "" {
				return "", errors.New("生成内容包含 SVG 以外的文本，请重试")
			}
		case xml.Comment:
			continue
		default:
			return "", errors.New("动画包含不允许的 XML 指令")
		}
		if err := encoder.EncodeToken(token); err != nil {
			return "", errors.New("动画编码失败")
		}
	}
	if roots != 1 || elements < 2 || depth != 0 {
		return "", errors.New("上游没有返回有效 SVG 动画，请重试")
	}
	if err := encoder.Flush(); err != nil {
		return "", err
	}
	if output.Len() > 24<<10 {
		return "", errors.New("生成的 SVG 超过 24 KB，请更换模型后重试")
	}
	return output.String(), nil
}
