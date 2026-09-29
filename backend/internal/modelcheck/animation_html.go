package modelcheck

import (
	"bytes"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
)

const animationPreviewCSP = "default-src 'none'; script-src 'none'; style-src 'unsafe-inline'; img-src data: blob:; media-src data: blob:; font-src data: blob:; connect-src 'none'; child-src 'none'; frame-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'"
const animationSourceLimit = 128 << 10

var animationCSSURLs = regexp.MustCompile(`(?i)url\s*\(\s*([^)]*)\)`)

func setAnimationArtifact(result *AnimationResult, source string) error {
	result.Source, result.SourceTruncated = boundedAnimationSource(source)
	trimmed := strings.TrimSpace(source)
	if strings.HasPrefix(trimmed, "```") {
		if first := strings.IndexByte(trimmed, '\n'); first >= 0 && strings.HasSuffix(trimmed, "```") {
			trimmed = strings.TrimSpace(trimmed[first+1 : len(trimmed)-3])
		}
	}
	var err error
	if strings.HasPrefix(strings.ToLower(trimmed), "<svg") {
		result.SVG, err = sanitizeAnimationSVG(trimmed)
	} else {
		result.HTML, err = sanitizeAnimationHTML(trimmed)
	}
	return err
}

func boundedAnimationSource(source string) (string, bool) {
	if len(source) <= animationSourceLimit {
		return source, false
	}
	limit := animationSourceLimit
	for limit > 0 && !utf8.ValidString(source[:limit]) {
		limit--
	}
	return source[:limit], true
}

// Parse the document before rendering in an opaque-origin iframe.
func sanitizeAnimationHTML(source string) (string, error) {
	if len(source) > 128<<10 {
		return "", errors.New("生成的 HTML 超过 128 KB，请重试")
	}
	lower := strings.ToLower(source)
	if !strings.Contains(lower, "<html") || !strings.HasSuffix(strings.TrimSpace(lower), "</html>") {
		return "", errors.New("生成内容不是完整的单文件 HTML 或 SVG，请重试")
	}
	document, err := html.Parse(strings.NewReader(source))
	if err != nil {
		return "", errors.New("生成的 HTML 无法解析，请重试")
	}
	elements := 0
	var head *html.Node
	var visit func(*html.Node, int) error
	visit = func(node *html.Node, depth int) error {
		if depth > 64 {
			return errors.New("生成的 HTML 结构过于复杂，请重试")
		}
		if node.Type == html.ElementNode {
			if node.Data == "script" {
				node.Parent.RemoveChild(node)
				return nil
			}
			attrs := node.Attr[:0]
			for _, attr := range node.Attr {
				if !strings.HasPrefix(strings.ToLower(attr.Key), "on") {
					attrs = append(attrs, attr)
				}
			}
			node.Attr = attrs
			elements++
			if elements > 4000 {
				return errors.New("生成的 HTML 元素过多，请重试")
			}
			if node.Data == "head" {
				head = node
			}
			if err := validateAnimationHTMLResources(node); err != nil {
				return err
			}
			if node.Data == "style" {
				for child := node.FirstChild; child != nil; child = child.NextSibling {
					if child.Type != html.TextNode {
						return errors.New("动画样式内容无效")
					}
					if err := validateAnimationCSSResources(child.Data); err != nil {
						return err
					}
				}
			}
		}
		for child := node.FirstChild; child != nil; {
			next := child.NextSibling
			if err := visit(child, depth+1); err != nil {
				return err
			}
			child = next
		}
		return nil
	}
	if err := visit(document, 0); err != nil {
		return "", err
	}
	if head == nil {
		return "", errors.New("生成内容不是完整的单文件 HTML，请重试")
	}
	head.InsertBefore(&html.Node{Type: html.ElementNode, Data: "meta", Attr: []html.Attribute{{Key: "http-equiv", Val: "Content-Security-Policy"}, {Key: "content", Val: animationPreviewCSP}}}, head.FirstChild)
	var output bytes.Buffer
	if err := html.Render(&output, document); err != nil {
		return "", errors.New("动画 HTML 编码失败")
	}
	return output.String(), nil
}

func validateAnimationHTMLResources(node *html.Node) error {
	for _, attr := range node.Attr {
		key := strings.ToLower(attr.Key)
		if key != "xmlns" && !strings.HasPrefix(key, "xmlns:") && containsExternalURL(attr.Val) {
			return errors.New("动画不能引用外部资源")
		}
		if key == "style" {
			if err := validateAnimationCSSResources(attr.Val); err != nil {
				return err
			}
			continue
		}
		if key == "src" || key == "srcset" || key == "href" || key == "xlink:href" || key == "poster" || key == "background" || key == "action" || key == "formaction" || key == "cite" || key == "usemap" || key == "manifest" {
			if err := validateAnimationResourceReference(attr.Val); err != nil {
				return err
			}
		}
		if node.Data == "meta" && key == "content" && strings.EqualFold(attrValue(node, "http-equiv"), "refresh") && containsExternalURL(attr.Val) {
			return errors.New("动画不能引用外部资源")
		}
	}
	return nil
}

func attrValue(node *html.Node, key string) string {
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, key) {
			return attr.Val
		}
	}
	return ""
}

func validateAnimationResourceReference(value string) error {
	value = strings.TrimSpace(value)
	lower := strings.ToLower(value)
	if value == "" || strings.HasPrefix(value, "#") || strings.HasPrefix(lower, "data:") || strings.HasPrefix(lower, "blob:") {
		return nil
	}
	return errors.New("动画不能引用外部资源")
}

func containsExternalURL(value string) bool {
	lower := strings.ToLower(value)
	return strings.Contains(lower, "http://") || strings.Contains(lower, "https://") || strings.Contains(lower, "//")
}

func validateAnimationCSSResources(value string) error {
	if containsExternalURL(value) {
		return errors.New("动画不能引用外部资源")
	}
	for _, match := range animationCSSURLs.FindAllStringSubmatch(value, -1) {
		if err := validateAnimationResourceReference(match[1]); err != nil {
			return err
		}
	}
	if strings.Contains(strings.ToLower(value), "@import") {
		return errors.New("动画样式不能引用外部资源")
	}
	return nil
}
