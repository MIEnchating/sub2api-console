// Package animationquality classifies generated source; it never executes it.
package animationquality

import (
	"crypto/sha256"
	"fmt"
)

const MaxSourceBytes = 128 << 10
const PelicanPrompt = `请生成可直接运行的单文件HTML，使用内联SVG绘制鹈鹕骑自行车的二维循环动画。画面以鹈鹕和自行车为主体，展示清晰的身体结构、踩踏动作和车轮转动，配合协调的背景、配色与层次。动画应流畅自然、衔接连续，并适配不同屏幕尺寸。可使用内联JavaScript脚本实现动画，但不得发送网络请求、加载外部脚本或引用外部资源，只输出完整HTML，不要代码围栏或解释文字。`

type Prediction struct {
	Source            string   `json:"source,omitempty"`
	Explanation       string   `json:"explanation,omitempty"`
	ReviewerAccountID string   `json:"reviewer_account_id,omitempty"`
	SettingsVersion   int      `json:"settings_version,omitempty"`
	ReviewedAt        string   `json:"reviewed_at,omitempty"`
	ResponseModel     string   `json:"response_model,omitempty"`
	Status            string   `json:"status"`
	Score             *float64 `json:"score"`
	Reason            string   `json:"reason,omitempty"`
	Benchmark         string   `json:"benchmark"`
	ModelVersion      string   `json:"model_version,omitempty"`
	ModelSHA256       string   `json:"model_sha256,omitempty"`
}

type Annotation struct {
	Label       string `json:"label"`
	Reason      string `json:"reason"`
	SourceGroup string `json:"source_group"`
	Version     int    `json:"version"`
	UpdatedAt   string `json:"updated_at"`
	UpdatedBy   string `json:"updated_by"`
}

// Samples live independently of task retention. No credentials or endpoint URLs
// are copied; neither model identity nor token usage supplies the human label.
type Sample struct {
	ID           string     `json:"id"`
	TaskID       string     `json:"task_id"`
	AccountID    string     `json:"account_id"`
	RequestID    string     `json:"request_id"`
	Benchmark    string     `json:"benchmark"`
	Prompt       string     `json:"prompt"`
	PromptSHA256 string     `json:"prompt_sha256"`
	HTML         string     `json:"html"`
	SourceSHA256 string     `json:"source_sha256"`
	Prediction   Prediction `json:"prediction"`
	Annotation   Annotation `json:"annotation"`
	CreatedAt    string     `json:"created_at"`
}

type SamplePage struct {
	Items      []Sample `json:"items"`
	NextCursor string   `json:"next_cursor,omitempty"`
}

func Hash(value string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(value))) }
func Benchmark(prompt string) string {
	if prompt == PelicanPrompt {
		return "pelican-v1"
	}
	if prompt == "" {
		return ""
	}
	return "prompt-" + Hash(prompt)
}
