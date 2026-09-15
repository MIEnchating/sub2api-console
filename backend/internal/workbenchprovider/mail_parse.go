package workbenchprovider

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"html"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	xhtml "golang.org/x/net/html"
)

var mailDigits = regexp.MustCompile(`[0-9]{6}`)
var mailCodeContext = regexp.MustCompile(`(?i)(openai|chatgpt|verif(?:ication|y)|security\s*code|one[- ]time|\botp\b|验证码|校验码|一次性)`)
var mailCodeField = regexp.MustCompile(`(?i)^(?:code|otp|verification_?code|verify_?code|security_?code|sms_?code|auth_?code|验证码|校验码)$`)
var mailTextTime = regexp.MustCompile(`20[0-9]{2}[-/.][0-9]{1,2}[-/.][0-9]{1,2}[T ][0-9]{1,2}:[0-9]{2}(?::[0-9]{2}(?:\.[0-9]{1,9})?)?(?:[ ]?(?:Z|[+-][0-9]{2}:?[0-9]{2}))?`)

type mailSource struct {
	field, text, messageID string
	receivedAt             time.Time
}

func ExtractMailCandidates(raw []byte) ([]MailCandidate, error) {
	if len(raw) > MaxResponseBytes {
		return nil, mailError("mail_response_too_large", "邮箱响应超过 2 MiB 限制")
	}
	input := bytes.TrimSpace(raw)
	if len(input) == 0 {
		return []MailCandidate{}, nil
	}
	var sources []mailSource
	var value any
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err == nil {
		var trailing any
		if decoder.Decode(&trailing) != io.EOF {
			if input[0] == '{' || input[0] == '[' || input[0] == '"' {
				return nil, mailError("mail_response_invalid", "邮箱接口 JSON 响应无效，请检查服务返回内容")
			}
			// A plain-text timestamp can begin with a valid JSON number prefix.
			sources = []mailSource{{text: string(input)}}
		} else {
			if failedMailEnvelope(value, 0) {
				return nil, mailError("mail_service_failed", "邮箱收码服务未成功处理请求，请检查账号权限和服务配置")
			}
			collectMailSources(value, mailSource{}, &sources, 0)
		}
	} else {
		if input[0] == '{' || input[0] == '[' {
			return nil, mailError("mail_response_invalid", "邮箱接口 JSON 响应无效，请检查服务返回内容")
		}
		sources = []mailSource{{text: string(input)}}
	}
	byKey := make(map[string]MailCandidate)
	for _, source := range sources {
		for _, variant := range mailTextVariants(source.text) {
			text := normalizeMailText(variant)
			var timestamps []mailTimestamp
			timestampsReady := false
			for _, index := range mailDigits.FindAllStringIndex(text, -1) {
				if (index[0] > 0 && asciiDigit(text[index[0]-1])) || (index[1] < len(text) && asciiDigit(text[index[1]])) {
					continue
				}
				code := text[index[0]:index[1]]
				nearby := text[max(0, index[0]-120):min(len(text), index[1]+120)]
				explicitCode := mailCodeField.MatchString(source.field)
				contextual := mailCodeContext.MatchString(nearby)
				if !explicitCode && !contextual {
					continue
				}
				score := 0
				if explicitCode {
					score += 40
				}
				if contextual {
					score += 40
				}
				if strings.TrimSpace(text) == code {
					score += 30
				}
				if len(text) <= 500 {
					score += 4
				}
				received := source.receivedAt
				if received.IsZero() {
					if !timestampsReady {
						timestamps = mailTimestamps(text)
						timestampsReady = true
					}
					received = nearbyMailTime(timestamps, index[0])
				}
				identity := "context:" + mailFingerprint(nearby)
				if !received.IsZero() {
					identity = "time:" + received.UTC().Format(time.RFC3339Nano)
				}
				if source.messageID != "" {
					identity = "id:" + source.messageID + "|time:" + received.UTC().Format(time.RFC3339Nano)
				}
				candidate := MailCandidate{Code: code, Key: mailFingerprint(code + "|" + identity), ReceivedAt: received, Score: score}
				if previous, exists := byKey[candidate.Key]; !exists || candidate.Score > previous.Score {
					byKey[candidate.Key] = candidate
				}
			}
		}
	}
	result := make([]MailCandidate, 0, len(byKey))
	for _, candidate := range byKey {
		result = append(result, candidate)
	}
	slices.SortFunc(result, func(a, b MailCandidate) int {
		if !a.ReceivedAt.IsZero() && !b.ReceivedAt.IsZero() && !a.ReceivedAt.Equal(b.ReceivedAt) {
			return b.ReceivedAt.Compare(a.ReceivedAt)
		}
		if a.Score != b.Score {
			return b.Score - a.Score
		}
		return strings.Compare(a.Key, b.Key)
	})
	return result, nil
}

func failedMailEnvelope(value any, depth int) bool {
	object, ok := value.(map[string]any)
	if !ok || depth > 3 {
		return false
	}
	for key, child := range object {
		switch strings.ToLower(key) {
		case "success", "ok":
			if child == false {
				return true
			}
		case "error":
			if mailEnvelopeHasError(child) {
				return true
			}
		case "status":
			if child == false {
				return true
			}
			if status, ok := child.(string); ok {
				switch strings.ToLower(status) {
				case "error", "failed", "failure", "unauthorized", "forbidden":
					return true
				}
			}
		case "code", "statuscode", "status_code":
			if status, ok := child.(json.Number); ok {
				code, _ := status.Int64()
				if code >= 400 && code <= 599 {
					return true
				}
			}
		case "data", "result", "response":
			if failedMailEnvelope(child, depth+1) {
				return true
			}
		}
	}
	return false
}

func mailEnvelopeHasError(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case bool:
		return typed
	case json.Number:
		return typed != "0"
	case string:
		text := strings.ToLower(strings.TrimSpace(typed))
		return text != "" && text != "0" && text != "false"
	default:
		return true
	}
}

func collectMailSources(value any, inherited mailSource, output *[]mailSource, depth int) {
	if depth > 32 || len(*output) >= 10000 {
		return
	}
	switch typed := value.(type) {
	case string:
		inherited.text = typed
		*output = append(*output, inherited)
	case json.Number:
		if mailCodeField.MatchString(inherited.field) {
			inherited.text = typed.String()
			*output = append(*output, inherited)
		}
	case []any:
		for _, child := range typed {
			collectMailSources(child, inherited, output, depth+1)
		}
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		timePriority := -1
		for _, key := range keys {
			canonical := strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(key))
			switch canonical {
			case "id", "uid", "uuid", "messageid", "mailid", "emailid":
				if text, ok := typed[key].(string); ok {
					inherited.messageID = text
				}
				if number, ok := typed[key].(json.Number); ok {
					inherited.messageID = number.String()
				}
			case "receivedat", "receiveddatetime", "receivedtime", "createdat", "createdtime", "deliveredat", "sentat", "sentdatetime", "arrival", "timestamp", "date", "time", "updatedat":
				priority := mailTimePriority(canonical)
				if priority > timePriority {
					inherited.receivedAt = parseMailTime(typed[key])
					timePriority = priority
				}
			}
		}
		for _, key := range keys {
			child := inherited
			child.field = key
			collectMailSources(typed[key], child, output, depth+1)
		}
	}
}

func parseMailTime(value any) time.Time {
	var text string
	switch typed := value.(type) {
	case string:
		text = strings.TrimSpace(typed)
	case json.Number:
		text = typed.String()
	default:
		return time.Time{}
	}
	var parsed time.Time
	if number, err := strconv.ParseInt(text, 10, 64); err == nil {
		if len(text) == 10 {
			parsed = time.Unix(number, 0)
		}
		if len(text) == 13 {
			parsed = time.UnixMilli(number)
		}
	} else {
		normalized := strings.ReplaceAll(text, "/", "-")
		if len(normalized) >= 10 && normalized[4] == '.' && normalized[7] == '.' {
			normalized = normalized[:4] + "-" + normalized[5:7] + "-" + normalized[8:]
		}
		for _, layout := range []string{time.RFC3339Nano, time.RFC1123Z, time.RFC1123, "2006-01-02 15:04:05Z07:00", "2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02T15:04:05", "2006-01-02T15:04"} {
			if result, err := time.Parse(layout, normalized); err == nil {
				parsed = result
				break
			}
		}
	}
	if parsed.Before(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)) || parsed.After(time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)) {
		return time.Time{}
	}
	return parsed.UTC()
}

type mailTimestamp struct {
	position int
	time     time.Time
}

func mailTimestamps(text string) []mailTimestamp {
	var timestamps []mailTimestamp
	for _, location := range mailTextTime.FindAllStringIndex(text, -1) {
		parsed := parseMailTime(text[location[0]:location[1]])
		if !parsed.IsZero() {
			timestamps = append(timestamps, mailTimestamp{position: location[0], time: parsed})
		}
	}
	return timestamps
}

func nearbyMailTime(timestamps []mailTimestamp, position int) time.Time {
	if len(timestamps) == 0 {
		return time.Time{}
	}
	index, _ := slices.BinarySearchFunc(timestamps, position, func(value mailTimestamp, target int) int {
		return value.position - target
	})
	if index == len(timestamps) || (index > 0 && position-timestamps[index-1].position <= timestamps[index].position-position) {
		return timestamps[index-1].time
	}
	return timestamps[index].time
}

func mailTextVariants(text string) []string {
	variants := []string{text}
	compact := strings.Join(strings.Fields(text), "")
	if len(compact) < 16 {
		return variants
	}
	decoded, err := base64.StdEncoding.DecodeString(compact)
	if err != nil {
		decoded, err = base64.RawURLEncoding.DecodeString(compact)
	}
	if err != nil || len(decoded) == 0 || !utf8.Valid(decoded) {
		return variants
	}
	printable, total := 0, 0
	for _, r := range string(decoded) {
		total++
		if unicode.IsPrint(r) || unicode.IsSpace(r) {
			printable++
		}
	}
	if printable*100 >= total*85 {
		variants = append(variants, string(decoded))
	}
	return variants
}

func normalizeMailText(text string) string {
	tokenizer := xhtml.NewTokenizer(strings.NewReader(html.UnescapeString(text)))
	var output strings.Builder
	suppressed := ""
	suppressedDepth := 0
	for {
		switch tokenizer.Next() {
		case xhtml.ErrorToken:
			return strings.Join(strings.Fields(output.String()), " ")
		case xhtml.StartTagToken:
			name, _ := tokenizer.TagName()
			tag := string(name)
			if suppressed != "" && tag == suppressed {
				suppressedDepth++
			}
			if suppressed == "" && (tag == "script" || tag == "style" || tag == "template") {
				suppressed = tag
				suppressedDepth = 1
			}
			output.WriteByte(' ')
		case xhtml.EndTagToken:
			name, _ := tokenizer.TagName()
			if string(name) == suppressed {
				suppressedDepth--
				if suppressedDepth == 0 {
					suppressed = ""
				}
			}
			output.WriteByte(' ')
		case xhtml.SelfClosingTagToken:
			output.WriteByte(' ')
		case xhtml.TextToken:
			if suppressed == "" {
				output.Write(tokenizer.Text())
			}
		}
	}
}

func asciiDigit(value byte) bool { return value >= '0' && value <= '9' }

func mailTimePriority(key string) int {
	if strings.HasPrefix(key, "received") {
		return 4
	}
	if strings.HasPrefix(key, "delivered") || key == "arrival" {
		return 3
	}
	if strings.HasPrefix(key, "sent") {
		return 2
	}
	if key == "updatedat" {
		return 0
	}
	return 1
}
