package modelcheck

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelfingerprint"
	"math/big"
	"strconv"
	"strings"
)

type verificationChallenge struct {
	public                      *modelfingerprint.Probe
	id, label, prompt, expected string
	jsonOutput                  bool
	measurement                 bool
	behavior                    bool
}

func verificationChallenges() ([]verificationChallenge, error) {
	values := make([]int, 4)
	for i := range values {
		value, err := rand.Int(rand.Reader, big.NewInt(80))
		if err != nil {
			return nil, err
		}
		values[i] = int(value.Int64()) + 12
	}
	nonce := make([]byte, 8)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	key := hex.EncodeToString(nonce)
	a, b, c, d := values[0], values[1], values[2], values[3]
	context := strings.Builder{}
	for i := 0; i < 48; i++ {
		if i == 23 {
			fmt.Fprintf(&context, "目标记录 %s 的校验码是 %d。\n", key, d*7)
		}
		fmt.Fprintf(&context, "普通记录 %d：库存 %d，批次 %d。\n", i, a+i, b+i)
	}
	payload, _ := json.Marshal(map[string]any{"code": key, "value": a + b})
	return []verificationChallenge{
		{id: "arithmetic", label: "多步计算", prompt: fmt.Sprintf("计算 (%d + %d) × %d - %d。只输出最终整数。", a, b, c, d), expected: strconv.Itoa((a+b)*c - d)},
		{id: "constraints", label: "约束推理", prompt: fmt.Sprintf("A、B、C 的总和为 %d。B 比 A 多 %d，C 是 A 的 2 倍。B 是多少？只输出整数。", 4*a+b, b), expected: strconv.Itoa(a + b)},
		{id: "state", label: "代码状态推演", prompt: fmt.Sprintf("执行整数伪代码：x=%d; 对 i 从 1 到 4（含端点）依次执行：若 i 为偶数则 x=x*2，否则 x=x+i。最终 x 是多少？只输出整数。", a), expected: strconv.Itoa(4*a + 10)},
		{id: "logic", label: "路径约束推理", prompt: fmt.Sprintf("有向图只有以下边：S→A 成本 %d，S→B 成本 %d，A→T 成本 %d，B→T 成本 %d，A→B 成本 3。边只能按箭头方向走。S 到 T 的最低总成本是多少？只输出整数。", a, b, d, c), expected: strconv.Itoa(min(a+d, b+c, a+3+c))},
		{id: "context", label: "上下文信息提取", prompt: context.String() + fmt.Sprintf("只输出目标记录 %s 的校验码，不输出其他记录的数值。", key), expected: strconv.Itoa(d * 7)},
		{id: "structured", label: "指令与结构化输出", prompt: fmt.Sprintf("只返回 JSON 对象，恰好包含 code（字符串 %s）和 value（整数 %d+%d 的结果）两个字段，不添加其他字段。", key, a, b), expected: string(payload), jsonOutput: true},
	}, nil
}

func verificationAnswerMatches(challenge verificationChallenge, answer string) bool {
	answer = strings.TrimSpace(answer)
	if strings.HasPrefix(answer, "```") && strings.HasSuffix(answer, "```") {
		if line := strings.IndexByte(answer, 10); line >= 0 {
			language := strings.ToLower(strings.TrimSpace(answer[3:line]))
			if language == "" || language == "json" || language == "text" {
				answer = strings.TrimSpace(answer[line+1 : len(answer)-3])
			}
		}
	}
	for _, marker := range []string{"***", "___", "**", "__", "*", "_", "`"} {
		if len(answer) > 2*len(marker) && strings.HasPrefix(answer, marker) && strings.HasSuffix(answer, marker) {
			inner := answer[len(marker) : len(answer)-len(marker)]
			if inner == challenge.expected {
				return true
			}
		}
	}
	if !challenge.jsonOutput {
		return answer == challenge.expected
	}
	var actual, expected map[string]json.RawMessage
	if json.Unmarshal([]byte(answer), &actual) != nil || json.Unmarshal([]byte(challenge.expected), &expected) != nil || len(actual) != len(expected) {
		return false
	}
	// Reject duplicate JSON keys as an ambiguous output contract.
	decoder := json.NewDecoder(strings.NewReader(answer))
	if _, err := decoder.Token(); err != nil {
		return false
	}
	seen := map[string]bool{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return false
		}
		name, ok := token.(string)
		if !ok || seen[name] {
			return false
		}
		seen[name] = true
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return false
		}
	}
	for name, value := range expected {
		if name == "code" {
			var want, got string
			if json.Unmarshal(value, &want) != nil || json.Unmarshal(actual[name], &got) != nil || want != got {
				return false
			}
		} else {
			var want, got int
			if json.Unmarshal(value, &want) != nil || json.Unmarshal(actual[name], &got) != nil || want != got {
				return false
			}
		}
	}
	return true
}
