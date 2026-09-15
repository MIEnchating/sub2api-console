package workbenchprovider

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

type smsCustomEntry struct {
	phone string
	url   string
}

// SMSPool shares one-time custom-number claims across authorization sessions.
// Only list and number hashes survive an individual SMS.Close call.
type SMSPool struct {
	mu     sync.Mutex
	groups map[[32]byte]map[[32]byte]bool
}

func NewSMSPool() *SMSPool { return &SMSPool{groups: make(map[[32]byte]map[[32]byte]bool)} }

func (p *SMSPool) claim(entries []smsCustomEntry) (int, error) {
	list := make([]string, len(entries))
	for index, entry := range entries {
		list[index] = entry.phone + "----" + entry.url
	}
	slices.Sort(list)
	raw, _ := json.Marshal(list)
	listID := sha256.Sum256(raw)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.groups == nil {
		p.groups = make(map[[32]byte]map[[32]byte]bool)
	}
	claimed := p.groups[listID]
	if claimed == nil {
		if len(p.groups) >= 1024 {
			return -1, smsFailure("sms_pool_limit", "接码号码批次已达到本次服务运行限制，请由管理员核对后重启服务", true)
		}
		claimed = make(map[[32]byte]bool)
		p.groups[listID] = claimed
	}
	for index, entry := range entries {
		phoneID := sha256.Sum256([]byte(entry.phone))
		if !claimed[phoneID] {
			claimed[phoneID] = true
			return index, nil
		}
	}
	return -1, smsFailure("sms_entries_exhausted", "本批自定义号码已全部分配，请补充新号码", true)
}

func parseSMSEntries(raw string) ([]smsCustomEntry, error) {
	if len(raw) > MaxResponseBytes {
		return nil, smsFailure("sms_entries_limit", "自定义号码列表超过 2 MiB 限制", true)
	}
	entries := []smsCustomEntry{}
	phones := map[string]bool{}
	for index, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		phone, endpoint, found := strings.Cut(line, "----")
		phone, endpoint = strings.TrimSpace(phone), strings.TrimSpace(endpoint)
		if !found || !smsPhone.MatchString(phone) || len(endpoint) > 8192 || ValidateURL(endpoint) != nil {
			return nil, smsFailure("sms_entry_invalid", fmt.Sprintf("第 %d 行需要国际格式手机号和公开 HTTPS 接码地址，以四个短横线分隔", index+1), true)
		}
		if phones[phone] {
			return nil, smsFailure("sms_entry_duplicate", fmt.Sprintf("第 %d 行手机号重复，请删除重复项", index+1), true)
		}
		phones[phone] = true
		entries = append(entries, smsCustomEntry{phone: phone, url: endpoint})
		if len(entries) > 500 {
			return nil, smsFailure("sms_entries_limit", "自定义接码一次最多导入 500 条号码", true)
		}
	}
	if len(entries) == 0 {
		return nil, smsFailure("sms_entries_empty", "请至少提供一条手机号与接码地址", true)
	}
	return entries, nil
}

func (s *SMS) customAcquire(ctx context.Context) (*smsActivation, error) {
	index, err := s.pool.claim(s.entries)
	if err != nil {
		return nil, err
	}
	selected := &s.entries[index]
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return nil, smsFailure("sms_request_failed", "接码请求 ID 创建失败，请重新授权", true)
	}
	candidates, err := s.customCandidates(ctx, selected.url)
	if err != nil {
		return nil, err
	}
	activation := &smsActivation{number: SMSNumber{RequestID: "custom-" + hex.EncodeToString(random), Phone: selected.phone}, custom: selected, acquiredAt: time.Now(), actions: make(map[string]error), seenKeys: make(map[string]bool), seenCodes: make(map[string]bool)}
	for _, candidate := range candidates {
		activation.seenKeys[candidate.Key], activation.seenCodes[candidate.Code] = true, true
	}
	return activation, nil
}

func (s *SMS) customPoll(ctx context.Context, activation *smsActivation) (SMSMessage, error) {
	candidates, err := s.customCandidates(ctx, activation.custom.url)
	if err != nil {
		return SMSMessage{}, err
	}
	result := SMSMessage{Pending: true}
	for _, candidate := range candidates {
		if !candidate.ReceivedAt.IsZero() && (candidate.ReceivedAt.Before(activation.acquiredAt.Truncate(time.Second)) || candidate.ReceivedAt.After(time.Now().Add(time.Minute))) {
			continue
		}
		if result.Code == "" && !activation.seenKeys[candidate.Key] && !activation.seenCodes[candidate.Code] {
			result.Pending, result.Code = false, candidate.Code
		}
		activation.seenKeys[candidate.Key], activation.seenCodes[candidate.Code] = true, true
	}
	if len(activation.seenKeys) > 2000 || len(activation.seenCodes) > 2000 {
		activation.seenKeys, activation.seenCodes = nil, nil
		activation.closed = true
		return SMSMessage{}, smsFailure("sms_custom_history_limit", "本次接码记录已超过限制，请重新授权并限制接口返回近期短信", true)
	}
	return result, nil
}

func (s *SMS) customCandidates(ctx context.Context, endpoint string) ([]MailCandidate, error) {
	body, err := s.http.Do(ctx, Request{Method: http.MethodGet, URL: endpoint})
	if err != nil {
		return nil, smsFailure("sms_custom_read_failed", "自定义接码地址读取失败，请检查地址权限和服务状态", false)
	}
	text := strings.TrimSpace(string(body))
	if len(text) == 6 && independentSMSCode(text) == text {
		return []MailCandidate{{Code: text, Key: mailFingerprint("sms-code:" + text)}}, nil
	}
	candidates, err := ExtractMailCandidates(body)
	if err != nil {
		return nil, smsFailure("sms_custom_response_invalid", "自定义接码响应无效，请检查服务返回内容", false)
	}
	if len(candidates) > 1000 {
		return nil, smsFailure("sms_custom_history_limit", "接码接口返回的验证码记录过多，请限制接口返回近期短信", true)
	}
	return candidates, nil
}
