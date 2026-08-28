package naming

import (
	"net"
	"net/url"
	"regexp"
	"strings"
)

var defaultSiteNamePattern = regexp.MustCompile(`[^a-z0-9]+`)

func IsDefaultSiteName(value string) bool {
	normalized := defaultSiteNamePattern.ReplaceAllString(strings.ToLower(strings.TrimSpace(value)), "")
	switch normalized {
	case "sub2api", "newapi", "oneapi":
		return true
	default:
		return false
	}
}

func DomainBrand(baseURL string) string {
	host := strings.TrimSpace(baseURL)
	if parsed, err := url.Parse(host); err == nil && parsed.Hostname() != "" {
		host = parsed.Hostname()
	} else if parsed, err := url.Parse("//" + host); err == nil && parsed.Hostname() != "" {
		host = parsed.Hostname()
	}
	host = strings.TrimSuffix(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(host)), "www."), ".")
	if host == "" || net.ParseIP(host) != nil {
		return "上游"
	}
	labels := strings.Split(host, ".")
	if len(labels) == 1 {
		return labels[0]
	}
	index := len(labels) - 2
	if len(labels) >= 3 && len(labels[len(labels)-1]) == 2 {
		switch labels[len(labels)-2] {
		case "com", "net", "org", "gov", "edu", "co":
			index = len(labels) - 3
		}
	}
	if labels[index] == "" {
		return "上游"
	}
	return labels[index]
}

func UpstreamName(siteName, baseURL string) string {
	name := strings.TrimSpace(siteName)
	if name != "" && !IsDefaultSiteName(name) {
		return name
	}
	return DomainBrand(baseURL)
}

func AccountName(siteName, baseURL, multiplier string) string {
	name := UpstreamName(siteName, baseURL)
	multiplier = strings.TrimSpace(multiplier)
	if multiplier == "" {
		return name
	}
	return name + "-" + multiplier
}
