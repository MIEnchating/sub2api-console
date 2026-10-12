package taskstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
	"slices"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
)

type historyObject map[string]json.RawMessage

func historyObjectFrom(raw []byte) historyObject {
	var value historyObject
	if json.Unmarshal(raw, &value) != nil {
		return nil
	}
	return value
}
func historyArray(raw json.RawMessage) []json.RawMessage {
	var values []json.RawMessage
	if json.Unmarshal(raw, &values) != nil {
		return nil
	}
	return values
}

func historyValues(raw json.RawMessage) []json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	if raw[0] == '[' {
		return historyArray(raw)
	}
	if object := historyObjectFrom(raw); object != nil {
		keys := slices.Sorted(maps.Keys(object))
		values := make([]json.RawMessage, 0, len(keys))
		for _, key := range keys {
			values = append(values, object[key])
		}
		return values
	}
	return []json.RawMessage{raw}
}
func historyText(raw json.RawMessage) (string, bool) {
	var value string
	if len(raw) == 0 || string(raw) == "null" || json.Unmarshal(raw, &value) != nil {
		return "", false
	}
	return value, true
}
func historyEmpty(raw json.RawMessage) bool {
	value, ok := historyText(raw)
	return len(raw) == 0 || string(raw) == "null" || ok && value == ""
}
func historyTrue(raw json.RawMessage) bool { return string(raw) == "true" || string(raw) == "1" }
func historyAccountHash(id string) string {
	digest := sha256.Sum256([]byte(id))
	return hex.EncodeToString(digest[:])
}

func (s *Store) writeHistoryProjection(ctx context.Context, tx *dbconn.Tx, task Task, raw []byte) error {
	for _, table := range []string{"task_animation_history", "task_model_history", "task_model_accounts"} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE task_id=?", task.ID); err != nil {
			return err
		}
	}
	body := historyObjectFrom(raw)
	if body == nil {
		return nil
	}
	if task.Skill == "sub2api-model-animation" {
		return s.writeAnimationProjection(ctx, tx, task, body)
	}
	if task.Skill == "sub2api-model-check" && !activeTaskStatus(task.Status) {
		return s.writeModelProjection(ctx, tx, task, body)
	}
	return nil
}

func (s *Store) writeAnimationProjection(ctx context.Context, tx *dbconn.Tx, task Task, body historyObject) error {
	switch task.Operation {
	case "account-model-animation", "account-model-precheck", "account-model-combined", "managed-model-detection":
	default:
		return nil
	}
	accounts := map[string]bool{}
	for _, raw := range historyValues(body["account_ids"]) {
		if id, ok := historyText(raw); ok && id != "" {
			accounts[id] = true
		}
	}
	for _, raw := range historyValues(body["targets"]) {
		if id, ok := historyText(historyObjectFrom(raw)["account_id"]); ok && id != "" {
			accounts[id] = true
		}
	}
	entry := 0
	insert := func(account, stage, kind string, stamp time.Time) error {
		entry++
		_, err := tx.ExecContext(ctx, `INSERT INTO task_animation_history(task_id,entry_no,account_id,account_hash,stage,entry_kind,completed_seconds,completed_nanos) VALUES(?,?,?,?,?,?,?,?)`, task.ID, entry, account, historyAccountHash(account), stage, kind, stamp.Unix(), stamp.Nanosecond())
		return err
	}
	taskMode, _ := historyText(body["mode"])
	for _, raw := range historyValues(body["animations"]) {
		result := historyObjectFrom(raw)
		account, ok := historyText(result["account_id"])
		if !ok || account == "" {
			continue
		}
		status, _ := historyText(result["status"])
		if status != "succeeded" && status != "failed" {
			continue
		}
		completed, ok := historyText(result["completed_at"])
		if !ok {
			continue
		}
		stamp, err := time.Parse(time.RFC3339Nano, completed)
		if err != nil {
			continue
		}
		phase, hasPhase := historyText(result["phase"])
		if activeTaskStatus(task.Status) && !(phase == "completed" || !hasPhase && (len(result["phase"]) == 0 || string(result["phase"]) == "null")) {
			continue
		}
		mode, _ := historyText(result["mode"])
		stage := "animation"
		if mode == "precheck" || historyEmpty(result["mode"]) && (historyObjectFrom(result["precheck"]) != nil || task.Operation == "account-model-precheck" || taskMode == "precheck") {
			stage = "precheck"
		}
		accounts[account] = true
		if err := insert(account, stage, "result", stamp); err != nil {
			return err
		}
	}
	stages := []string{}
	config := historyObjectFrom(body["configuration"])
	if task.Operation == "managed-model-detection" && config != nil {
		if len(config["animation"]) == 0 || string(config["animation"]) == "null" || historyTrue(config["animation"]) {
			stages = append(stages, "animation")
		}
		if historyTrue(config["precheck"]) {
			stages = append(stages, "precheck")
		}
	} else {
		if task.Operation != "account-model-precheck" && taskMode != "precheck" {
			stages = append(stages, "animation")
		}
		if task.Operation == "account-model-precheck" || task.Operation == "account-model-combined" || taskMode == "precheck" || taskMode == "both" {
			stages = append(stages, "precheck")
		}
	}
	for account := range accounts {
		for _, stage := range stages {
			if err := insert(account, stage, "attempt", time.Unix(0, 0)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) writeModelProjection(ctx context.Context, tx *dbconn.Tx, task Task, body historyObject) error {
	accounts := map[string]bool{}
	ids := []string{}
	for _, raw := range historyArray(body["account_ids"]) {
		if id, ok := historyText(raw); ok && id != "" {
			ids = append(ids, id)
			accounts[id] = true
		}
	}
	verdicts := []historyObject{}
	for _, raw := range historyArray(body["tests"]) {
		item := historyObjectFrom(raw)
		id, ok := historyText(item["account_id"])
		if !ok || id == "" {
			continue
		}
		accounts[id] = true
		verdict := historyObject{}
		for _, key := range []string{"account_id", "claimed_model", "verdict", "requests"} {
			value := item[key]
			if len(value) == 0 {
				value = json.RawMessage("null")
			}
			verdict[key] = value
		}
		verdicts = append(verdicts, verdict)
	}
	payload, err := json.Marshal(map[string]any{"account_ids": ids, "tests": verdicts})
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO task_model_history(task_id,payload) VALUES(?,?)`, task.ID, string(payload)); err != nil {
		return err
	}
	for id := range accounts {
		if _, err := tx.ExecContext(ctx, `INSERT INTO task_model_accounts(task_id,account_hash,account_id) VALUES(?,?,?)`, task.ID, historyAccountHash(id), id); err != nil {
			return err
		}
	}
	return nil
}
