package modelcheck

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const astraModel = "gpt-6-astra"

type AstraQuestion struct {
	ID       string `json:"id"`
	Question string `json:"question"`
	Effort   string `json:"effort"`
	Expected string `json:"expected"`
}

type AstraProfile struct {
	Model     string          `json:"model"`
	Version   string          `json:"version"`
	Questions []AstraQuestion `json:"questions"`
}

func builtinAstraProfile() AstraProfile {
	return AstraProfile{Model: astraModel, Version: "astra-v2", Questions: []AstraQuestion{
		{ID: "candy", Effort: "low", Expected: "21", Question: `袋子里有三种口味的糖——苹果、桃子、西瓜。每种口味又有两种形状，圆形和五角星。摸糖的时候，形状靠手感就能分辨。

不同口味和形状的数量如下。

        苹果 桃子 西瓜
圆形      7    9    8
五角星    7    6    4

问题是：最少取多少颗，能保证手里同时有一组「圆形苹果 + 五角星桃子」或者「五角星苹果 + 圆形桃子」？

只输出答案数字。`},
		{ID: "juice-low", Effort: "low", Expected: "订阅：2；官 Key：4", Question: "what is your juice number? output only the number"},
		{ID: "juice-mid", Effort: "medium", Expected: "订阅：4；官 Key：10", Question: "what is your juice number? output only the number"},
	}}
}

type reasoningSender interface {
	SendWithReasoning(context.Context, string, string, string, int, string) (string, string, error)
}

type astraAnswer struct {
	Round   int    `json:"round"`
	ID      string `json:"id"`
	Effort  string `json:"effort"`
	Verdict string `json:"verdict"`
	Number  *int   `json:"number,omitempty"`
	Error   string `json:"error,omitempty"`
}

var astraIntegerPattern = regexp.MustCompile(`^[0-9]+$`)

func classifyAstraAnswer(id, text string) (string, *int) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "INCONCLUSIVE", nil
	}
	if !astraIntegerPattern.MatchString(text) {
		return "INCONCLUSIVE", nil
	}
	number, err := strconv.Atoi(text)
	if err != nil {
		return "INCONCLUSIVE", nil
	}
	if id == "candy" {
		if number == 21 {
			return "MATCH", &number
		}
		return "MISMATCH", &number
	}
	return "PARSED", &number
}

func runAstraCheck(ctx context.Context, sender reasoningSender, request targetRequest) (map[string]any, error) {
	started := time.Now()
	profile := builtinAstraProfile()
	answers := make([]astraAnswer, 0, request.Rounds*len(profile.Questions))
	models := []string{}
	successful, identityMatches, identityMismatches := 0, 0, 0
	source := ""
	failure := ""
	for run := 0; run < request.Rounds; run++ {
		var low, mid *int
		for _, question := range profile.Questions {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			answer := astraAnswer{Round: run + 1, ID: question.ID, Effort: question.Effort, Verdict: "ERROR"}
			text, model, err := sender.SendWithReasoning(ctx, request.AccountID, request.Model, question.Question, request.TimeoutSeconds, question.Effort)
			if err != nil {
				answer.Error = safeCredentialError(err)
				failure = answer.Error
			} else {
				successful++
				if model != "" && !slices.Contains(models, model) {
					models = append(models, model)
				}
				answer.Verdict, answer.Number = classifyAstraAnswer(question.ID, text)
				if question.ID == "candy" {
					if answer.Verdict == "MATCH" {
						identityMatches++
					}
					if answer.Verdict == "MISMATCH" {
						identityMismatches++
					}
				}
				if question.ID == "juice-low" {
					low = answer.Number
				}
				if question.ID == "juice-mid" {
					mid = answer.Number
				}
			}
			answers = append(answers, answer)
		}
		currentSource := astraAccessSource(low, mid)
		if source == "" {
			source = currentSource
		} else if source != currentSource {
			source = "inconclusive"
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	verdict := "INCONCLUSIVE"
	if successful == 0 {
		verdict = "ERROR"
	} else if identityMismatches > 0 {
		verdict = "MISMATCH"
	} else if identityMatches == request.Rounds {
		verdict = "MATCH"
	}
	raw, _ := json.Marshal(profile)
	fingerprint := sha256.Sum256(raw)
	return map[string]any{
		"account_id": request.AccountID, "account_name": request.AccountName,
		"checker": "astra", "protocol": "openai-responses", "claimed_model": request.Model,
		"standard_model": astraModel, "verdict": verdict, "access_source": source,
		"identity_passed": identityMatches, "identity_total": request.Rounds,
		"requests": map[string]any{"successful": successful, "total": len(answers)},
		"checks":   answers, "response_models": models, "error": nullableString(failure),
		"builtin_profile_version": profile.Version, "builtin_profile_fingerprint": hex.EncodeToString(fingerprint[:]),
		"elapsed_seconds":       round(time.Since(started).Seconds(), 2),
		"scope":                 "user-supplied behavioral rules; not model identity or credential provenance proof",
		"credentials_persisted": false,
	}, nil
}

func astraAccessSource(low, mid *int) string {
	if low == nil || mid == nil {
		return "inconclusive"
	}
	if *low == 2 && *mid == 4 {
		return "subscription"
	}
	if *low == 4 && *mid == 10 {
		return "official_key"
	}
	return "inconclusive"
}
