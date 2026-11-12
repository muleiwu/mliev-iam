package service

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

// CheckProfileSession rejects a changed/consumed authorization transaction.
// The expected snapshot comes from the browser-bound session, never the request body.
func (s *OIDCSessionService) CheckProfileSession(ctx context.Context, expected *OIDCSession) error {
	current, err := s.GetSession(ctx, expected.SessionID)
	if err != nil {
		return err
	}
	if !sameProfileSession(current, expected) {
		return ErrProfileCompletionConflict
	}
	return nil
}

func sameProfileSession(current, expected *OIDCSession) bool {
	if current == nil || expected == nil || !current.IsAuthenticated || current.UserID == nil || current.IsConsentGranted || current.Error != nil || current.PendingTwoFactorID != "" {
		return false
	}
	a, errA := json.Marshal(current)
	b, errB := json.Marshal(expected)
	return errA == nil && errB == nil && bytes.Equal(a, b)
}

// RebindProfileSession changes only the login binding, with optimistic concurrency
// and the original Redis TTL. It cannot resurrect a deleted or switched session.
func (s *OIDCSessionService) RebindProfileSession(ctx context.Context, expected *OIDCSession, newSID string) error {
	if newSID == "" {
		return ErrProfileCompletionConflict
	}
	key := s.keyPrefix + expected.SessionID
	return s.redis.Watch(ctx, func(tx *redis.Tx) error {
		raw, err := tx.Get(ctx, key).Bytes()
		if err != nil {
			return err
		}
		var current OIDCSession
		if err := json.Unmarshal(raw, &current); err != nil {
			return err
		}
		if !sameProfileSession(&current, expected) || !current.ExpiresAt.After(time.Now()) {
			return ErrProfileCompletionConflict
		}
		ttl, err := tx.PTTL(ctx, key).Result()
		if err != nil {
			return err
		}
		if ttl <= 0 {
			return ErrProfileCompletionConflict
		}
		current.LoginSessionID = newSID
		payload, err := json.Marshal(&current)
		if err != nil {
			return err
		}
		_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.SetArgs(ctx, key, payload, redis.SetArgs{KeepTTL: true})
			return nil
		})
		return err
	}, key)
}
