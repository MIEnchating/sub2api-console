package uptimekuma

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func (s *Service) metrics(ctx context.Context, cfg configstore.UptimeKumaConfig) ([]Monitor, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.BaseURL+"/metrics", nil)
	if err != nil {
		return nil, protocolError()
	}
	req.SetBasicAuth("", cfg.APIKey)
	req.Header.Set("Accept", "text/plain; version=0.0.4")
	res, err := s.client.Do(req)
	if err != nil {
		return nil, failure("kuma_connection_failed", "无法读取 Uptime Kuma 指标，请检查服务地址、网络和 TLS 证书", 502)
	}
	defer res.Body.Close()
	if res.StatusCode == 401 || res.StatusCode == 403 {
		return nil, failure("kuma_api_key_rejected", "API 密钥无效、已过期或未启用，请在 Uptime Kuma 中检查密钥", 422)
	}
	if res.StatusCode != 200 {
		return nil, failure("kuma_metrics_unavailable", "指标接口不可用，请填写 Uptime Kuma 服务地址并检查 /metrics 接口", 502)
	}
	return parseMetrics(res.Body)
}
func parseMetrics(body io.Reader) ([]Monitor, error) {
	// Parse directly from the bounded response instead of retaining a second
	// complete copy alongside Prometheus' metric families.
	limited := &io.LimitedReader{R: body, N: (8 << 20) + 1}
	reader := &metricsHelpReader{Reader: limited}
	parser := expfmt.NewTextParser(model.UTF8Validation)
	families, err := parser.TextToMetricFamilies(reader)
	if err != nil || limited.N == 0 {
		return nil, protocolError()
	}
	// An empty Kuma instance still exposes the monitor_status gauge declaration.
	if _, ok := families["monitor_status"]; !ok && !reader.found {
		return nil, protocolError()
	}
	items := map[string]*Monitor{}
	for _, metricName := range []string{"monitor_status", "monitor_response_time", "monitor_cert_days_remaining", "monitor_uptime_ratio"} {
		family := families[metricName]
		if family == nil {
			continue
		}
		for _, metric := range family.Metric {
			var rawID, name, kind, url, hostname, port, window string
			for _, l := range metric.Label {
				switch l.GetName() {
				case "monitor_id":
					rawID = l.GetValue()
				case "monitor_name":
					name = l.GetValue()
				case "monitor_type":
					kind = l.GetValue()
				case "monitor_url":
					url = l.GetValue()
				case "monitor_hostname":
					hostname = l.GetValue()
				case "monitor_port":
					port = l.GetValue()
				case "window":
					window = l.GetValue()
				}
			}
			if name == "" || kind == "" || metric.Gauge == nil {
				return nil, protocolError()
			}
			id := int64(0)
			if rawID != "" {
				id, err = strconv.ParseInt(rawID, 10, 64)
				if err != nil || id <= 0 {
					return nil, protocolError()
				}
			}
			key := "id:" + strconv.FormatInt(id, 10)
			if id == 0 {
				sum := sha256.Sum256([]byte(strings.Join([]string{name, kind, url, hostname, port}, "\x00")))
				key = "metric:" + hex.EncodeToString(sum[:])
			}
			item := items[key]
			if item == nil {
				u, redacted := publicURL(url)
				item = &Monitor{ID: id, Key: key, Name: name, Type: kind, URL: u, URLRedacted: redacted, Active: true}
				items[key] = item
			}
			v := metric.Gauge.GetValue()
			if math.IsNaN(v) || math.IsInf(v, 0) {
				continue
			}
			switch metricName {
			case "monitor_status":
				if v < 0 || v > 3 || v != math.Trunc(v) {
					return nil, protocolError()
				}
				status := int(v)
				item.Status = &status
			case "monitor_response_time":
				if v >= 0 {
					item.ResponseTime = &v
				}
			case "monitor_cert_days_remaining":
				item.CertificateDays = &v
			case "monitor_uptime_ratio":
				if window == "1d" && v >= 0 && v <= 1 {
					item.Uptime = &v
				}
			}
		}
	}
	result := make([]Monitor, 0, len(items))
	for _, item := range items {
		result = append(result, *item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Key < result[j].Key })
	return result, nil
}

// expfmt removes empty metric families. Retain only whether the existing empty
// instance marker was seen, including when it spans reads, without saving the
// response. All protocol validation still belongs to the Prometheus parser.
type metricsHelpReader struct {
	io.Reader
	matched int
	found   bool
}

func (r *metricsHelpReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if r.found {
		return n, err
	}
	const marker = "# HELP monitor_status "
	for _, c := range p[:n] {
		if c == marker[r.matched] {
			r.matched++
		} else if c == marker[0] {
			// The marker contains no other '#', so only its first byte can
			// start an overlapping match.
			r.matched = 1
		} else {
			r.matched = 0
		}
		if r.matched == len(marker) {
			r.found = true
			break
		}
	}
	return n, err
}
