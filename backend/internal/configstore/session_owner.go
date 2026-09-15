package configstore

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

func (s *Store) ActiveSessionOwner(ctx context.Context, owner string, now time.Time) (bool, error) {
	expires, err := s.SessionOwnerExpires(ctx, owner)
	return err == nil && expires.After(now.UTC()), err
}

func (s *Store) SessionOwnerExpires(ctx context.Context, owner string) (time.Time, error) {
	if len(owner) != 64 || strings.Trim(owner, "0123456789abcdef") != "" {
		return time.Time{}, nil
	}
	var username, raw string
	err := s.db.QueryRowContext(ctx, `SELECT username,expires_at FROM console_sessions WHERE token_hash=?`, owner).Scan(&username, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	if strings.TrimSpace(username) == "" {
		return time.Time{}, nil
	}
	expires, err := parseTime(raw)
	if err != nil {
		return time.Time{}, nil
	}
	return expires, nil
}

// Capture this channel before checking the owner so revocation cannot be
// missed between validation and waiting. It contains no session credentials.
func (s *Store) SessionOwnerChanges() <-chan struct{} {
	s.sessionOwnerMu.Lock()
	defer s.sessionOwnerMu.Unlock()
	if s.sessionOwnerChanges == nil {
		s.sessionOwnerChanges = make(chan struct{})
	}
	return s.sessionOwnerChanges
}

func (s *Store) notifySessionOwners() {
	s.sessionOwnerMu.Lock()
	defer s.sessionOwnerMu.Unlock()
	if s.sessionOwnerChanges != nil {
		close(s.sessionOwnerChanges)
	}
	s.sessionOwnerChanges = make(chan struct{})
}
