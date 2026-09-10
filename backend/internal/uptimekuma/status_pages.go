package uptimekuma

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func statusPageConfig(raw map[string]json.RawMessage) *StatusPageConfig {
	p := &StatusPageConfig{Title: rawString(raw, "title"), Slug: rawString(raw, "slug"), Description: rawString(raw, "description"), Theme: rawString(raw, "theme"), Footer: rawString(raw, "footerText"), ShowTags: rawBool(raw, "showTags"), ShowPoweredBy: rawBool(raw, "showPoweredBy"), ShowCertificateExpiry: rawBool(raw, "showCertificateExpiry"), Domains: []string{}, Groups: []PublicGroup{}}
	_ = json.Unmarshal(raw["domainNameList"], &p.Domains)
	return p
}
func (s *Service) publicGroups(ctx context.Context, base, slug string) ([]PublicGroup, error) {
	if !slugPattern.MatchString(slug) {
		return nil, protocolError()
	}
	req, err := http.NewRequestWithContext(ctx, "GET", base+"/api/status-page/"+url.PathEscape(slug), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cache-Control", "no-cache")
	res, err := s.client.Do(req)
	if err != nil {
		return nil, failure("kuma_status_page_unavailable", "状态页分组读取失败，请检查连接后重试", 502)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, failure("kuma_status_page_unavailable", "状态页分组读取失败，请检查状态页是否可访问", 502)
	}
	var body struct {
		Groups []PublicGroup `json:"publicGroupList"`
	}
	if json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&body) != nil || body.Groups == nil {
		return nil, protocolError()
	}
	return body.Groups, nil
}
func validateStatusPage(p *StatusPageConfig, current map[string]json.RawMessage, monitors map[string]json.RawMessage) error {
	bad := func(msg string) error { return failure("kuma_invalid_status_page", msg, 422) }
	if p == nil || strings.TrimSpace(p.Title) == "" || len(p.Title) > 150 || len(p.Description) > 10000 || len(p.Footer) > 10000 || len(p.Slug) > 100 || !slugPattern.MatchString(p.Slug) {
		return bad("请检查状态页标题和路径（仅小写字母、数字与短横线）")
	}
	if current != nil && rawString(current, "slug") != p.Slug {
		return bad("编辑时保留原路径，以免已有状态页链接失效")
	}
	if p.Theme != "auto" && p.Theme != "light" && p.Theme != "dark" {
		return bad("状态页主题无效")
	}
	if len(p.Groups) > 100 || len(p.Domains) > 30 {
		return bad("状态页分组或域名数量过多")
	}
	for _, d := range p.Domains {
		if !validHost(d) {
			return bad("请填写不含协议和路径的域名")
		}
	}
	ids := []int64{}
	for _, g := range p.Groups {
		if strings.TrimSpace(g.Name) == "" || len(g.Name) > 150 {
			return bad("请填写展示分组名称")
		}
		for _, m := range g.MonitorList {
			ids = append(ids, m.ID)
			if m.URL != "" && !validateHTTPURL(m.URL) {
				return bad("监控自定义链接无效")
			}
		}
	}
	return validateObjectIDs(ids, monitors, false)
}
func statusPagePayload(p *StatusPageConfig, raw map[string]json.RawMessage) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	for k, v := range raw {
		out[k] = v
	}
	for k, v := range map[string]any{"slug": p.Slug, "title": strings.TrimSpace(p.Title), "description": p.Description, "theme": p.Theme, "footerText": p.Footer, "showTags": p.ShowTags, "showPoweredBy": p.ShowPoweredBy, "showCertificateExpiry": p.ShowCertificateExpiry, "domainNameList": p.Domains} {
		out[k], _ = json.Marshal(v)
	}
	return out
}
func preservePublicLinks(groups, old []PublicGroup) ([]PublicGroup, error) {
	existing := map[int64]PublicGroup{}
	for _, g := range old {
		existing[g.ID] = g
	}
	seen := map[int64]bool{}
	for i := range groups {
		g := &groups[i]
		if g.ID == 0 {
			continue
		}
		prev, ok := existing[g.ID]
		if !ok || seen[g.ID] {
			return nil, failure("kuma_invalid_status_page", "展示分组 ID 无效或重复，请刷新后重试", 422)
		}
		seen[g.ID] = true
		links := map[int64]string{}
		for _, m := range prev.MonitorList {
			links[m.ID] = m.URL
		}
		for j := range g.MonitorList {
			if g.MonitorList[j].URL == "" {
				g.MonitorList[j].URL = links[g.MonitorList[j].ID]
			}
		}
	}
	return groups, nil
}
