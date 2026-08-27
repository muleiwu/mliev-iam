package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type memoryAdminAuthStore struct {
	mu     sync.Mutex
	values map[string][]byte
}

func (s *memoryAdminAuthStore) Put(_ context.Context, key string, value []byte, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.values == nil {
		s.values = map[string][]byte{}
	}
	s.values[key] = append([]byte(nil), value...)
	return nil
}

func (s *memoryAdminAuthStore) Get(_ context.Context, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[key]
	if !ok {
		return nil, errors.New("not found")
	}
	return append([]byte(nil), value...), nil
}

func (s *memoryAdminAuthStore) GetDel(_ context.Context, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[key]
	if !ok {
		return nil, errors.New("not found")
	}
	delete(s.values, key)
	return append([]byte(nil), value...), nil
}

func TestAdminAuthFlowBindsBrowserAndConsumesStateOnce(t *testing.T) {
	store := &memoryAdminAuthStore{}
	flow := &AdminAuthFlowService{store: store, ttl: time.Minute}
	transaction, browserToken, err := flow.Start(context.Background(), "https://iam.example/api/admin/auth/callback", "/settings")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if len(transaction.CodeVerifier) < 43 || len(transaction.CodeChallenge) != 43 || transaction.State == "" || transaction.Nonce == "" {
		t.Fatalf("invalid transaction entropy/PKCE: %+v", transaction)
	}
	if _, err := flow.Consume(context.Background(), transaction.State, "wrong-browser"); err == nil {
		t.Fatal("wrong browser binding should be rejected")
	}
	consumed, err := flow.Consume(context.Background(), transaction.State, browserToken)
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	if consumed.ReturnTo != "/settings" || consumed.CodeVerifier != transaction.CodeVerifier {
		t.Fatalf("unexpected consumed transaction: %+v", consumed)
	}
	if _, err := flow.Consume(context.Background(), transaction.State, browserToken); err == nil {
		t.Fatal("replayed state should be rejected")
	}
}
