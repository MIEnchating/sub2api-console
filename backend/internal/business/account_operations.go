package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type AccountOperation struct {
	OperationID       string
	OperationType     string
	State             string
	Phase             string
	Actor             string
	Error             *string
	RemoteConfirmed   bool
	ReadbackConfirmed bool
	ObjectID          string
	ObjectName        *string
	GroupNames        []string
	FieldName         *string
	Before            any
	After             any
	Writeback         bool
}

func (s *Store) CommitAccountFieldsReadback(
	ctx context.Context,
	accountID string,
	name *string,
	priority *int64,
	loadFactor *string,
	concurrency *int64,
	multiplier *string,
	notesPresent bool,
	notes *string,
	operation AccountOperation,
) error {
	return s.commitAccountMutation(ctx, accountID, operation, func(tx *sql.Tx, now string) error {
		updates := make([]string, 0, 7)
		arguments := make([]any, 0, 9)
		if name != nil {
			updates = append(updates, "name=?")
			arguments = append(arguments, *name)
		}
		if priority != nil {
			updates = append(updates, "priority=?")
			arguments = append(arguments, *priority)
		}
		if loadFactor != nil {
			updates = append(updates, "load_factor=?")
			arguments = append(arguments, *loadFactor)
		}
		if concurrency != nil {
			updates = append(updates, "concurrency=?")
			arguments = append(arguments, *concurrency)
		}
		if multiplier != nil {
			updates = append(updates, "multiplier=?")
			arguments = append(arguments, *multiplier)
		}
		if notesPresent {
			var raw string
			if err := tx.QueryRowContext(ctx, `SELECT metadata_json FROM accounts WHERE id=?`, accountID).Scan(&raw); err != nil {
				return err
			}
			metadata, err := decodeJSONObject(raw)
			if err != nil {
				return errors.New("账号元数据损坏，无法保存备注读回")
			}
			if notes == nil {
				metadata["notes"] = nil
			} else {
				metadata["notes"] = *notes
			}
			encoded, err := json.Marshal(metadata)
			if err != nil {
				return err
			}
			updates = append(updates, "metadata_json=?")
			arguments = append(arguments, string(encoded))
		}
		if len(updates) == 0 {
			return errors.New("至少提供一个需要同步的账号字段")
		}
		updates = append(updates, "updated_at=?")
		arguments = append(arguments, now, accountID)
		if _, err := tx.ExecContext(ctx, `UPDATE accounts SET `+strings.Join(updates, ",")+` WHERE id=?`, arguments...); err != nil {
			return err
		}
		if multiplier != nil {
			if _, err := tx.ExecContext(ctx, `UPDATE account_groups SET group_rate=? WHERE account_id=?`, *multiplier, accountID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE bindings SET local_rate=?,updated_at=? WHERE local_account_id=?`, *multiplier, now, accountID); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) RecordAccountOperation(ctx context.Context, operation AccountOperation) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := insertAccountOperation(ctx, tx, operation); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SaveAccountModels(ctx context.Context, accountID string, models []string) error {
	if !positiveNumericID(accountID) {
		return errors.New("账号必须使用有效的稳定 ID")
	}
	normalized := make([]string, 0, len(models))
	seen := map[string]struct{}{}
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		key := strings.ToLower(model)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, model)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var raw string
	if err := tx.QueryRowContext(ctx, `SELECT metadata_json FROM accounts WHERE id=?`, accountID).Scan(&raw); err != nil {
		return err
	}
	metadata, err := decodeJSONObject(raw)
	if err != nil {
		return errors.New("账号元数据损坏，无法保存可用模型")
	}
	metadata["known_models"] = normalized
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE accounts SET metadata_json=?,updated_at=? WHERE id=?`,
		string(encoded), time.Now().UTC().Format(time.RFC3339Nano), accountID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) commitAccountMutation(
	ctx context.Context,
	accountID string,
	operation AccountOperation,
	mutate func(*sql.Tx, string) error,
) error {
	if !positiveNumericID(accountID) {
		return errors.New("账号必须使用有效的稳定 ID")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM accounts WHERE id=?`, accountID).Scan(&exists); err != nil {
		return err
	}
	if exists != 1 {
		return sql.ErrNoRows
	}
	if err := mutate(tx, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	if err := insertAccountOperation(ctx, tx, operation); err != nil {
		return err
	}
	return tx.Commit()
}

func insertAccountOperation(ctx context.Context, tx *sql.Tx, operation AccountOperation) error {
	objectIDValid := positiveNumericID(operation.ObjectID)
	if operation.OperationType == "account.onboarding" && operation.State == "failed" && strings.TrimSpace(operation.ObjectID) == "" {
		objectIDValid = true
	}
	if strings.TrimSpace(operation.OperationID) == "" || strings.TrimSpace(operation.OperationType) == "" ||
		strings.TrimSpace(operation.State) == "" || strings.TrimSpace(operation.Phase) == "" || !objectIDValid {
		return errors.New("账号操作审计字段不完整")
	}
	groups, err := json.Marshal(operation.GroupNames)
	if err != nil {
		return err
	}
	before, err := accountOperationJSON(operation.Before)
	if err != nil {
		return err
	}
	after, err := accountOperationJSON(operation.After)
	if err != nil {
		return err
	}
	var minimum sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MIN(source_id) FROM operation_audit WHERE source_id < 0`).Scan(&minimum); err != nil {
		return err
	}
	sourceID := int64(-1)
	if minimum.Valid && minimum.Int64 <= -1 {
		sourceID = minimum.Int64 - 1
	}
	var objectID any
	if strings.TrimSpace(operation.ObjectID) != "" {
		objectID = operation.ObjectID
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO operation_audit(
		source_id,operation_id,operation_type,state,phase,request_id,actor,source,error,
		remote_confirmed,readback_confirmed,object_type,object_id,object_name,group_names_json,
		field_name,before_json,after_json,writeback,created_at
	) VALUES(?,?,?,?,?,NULL,?,?,?,?,?,'account',?,?,?,?,?,?,?,?)`,
		sourceID, operation.OperationID, operation.OperationType, operation.State, operation.Phase,
		strings.TrimSpace(operation.Actor), "console", managementNullableString(operation.Error),
		operation.RemoteConfirmed, operation.ReadbackConfirmed, objectID,
		managementNullableString(operation.ObjectName), string(groups), managementNullableString(operation.FieldName),
		before, after, operation.Writeback, time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

func accountOperationJSON(value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return string(encoded), nil
}
