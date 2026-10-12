package business

import (
	"context"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// AddAccountModelBlock reads, merges and audits using the single writer transaction.
// A browser must never implement an append by replacing a previously read list.
func (s *Store) AddAccountModelBlock(ctx context.Context, model, actor string) (AccountModelSyncSettings, error) {
	model = strings.TrimSpace(model)
	if model == "" || utf8.RuneCountInString(model) > 256 || strings.ContainsAny(model, "*?") || strings.IndexFunc(model, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return AccountModelSyncSettings{}, errors.New("屏蔽模型必须是不含空白和通配符的具体模型名称，且不能超过 256 个字符")
	}
	tx, err := s.beginPolicyTx(ctx, "control-plane")
	if err != nil {
		return AccountModelSyncSettings{}, err
	}
	defer tx.Rollback()
	document, err := s.readPolicyDocument(ctx, tx, "control-plane")
	if err != nil {
		return AccountModelSyncSettings{}, err
	}
	patterns, err := modelBlockPatternsFromDocument(document)
	if err != nil {
		return AccountModelSyncSettings{}, err
	}
	if ModelMatchesBlockPatterns(model, patterns) {
		return AccountModelSyncSettings{BlockedPatterns: patterns}, nil
	}
	patterns, err = validatedModelBlockPatterns(append(patterns, model))
	if err != nil {
		return AccountModelSyncSettings{}, err
	}
	if err := s.writeModelBlockPatterns(ctx, tx, document, patterns, actor); err != nil {
		return AccountModelSyncSettings{}, err
	}
	if err := tx.Commit(); err != nil {
		return AccountModelSyncSettings{}, err
	}
	return AccountModelSyncSettings{BlockedPatterns: patterns}, nil
}
