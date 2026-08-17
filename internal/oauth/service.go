// Package oauth owns Wahoo target authorization use cases.
package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"
)

const stateLifetime = 10 * time.Minute

var (
	// ErrInvalidAuthorization is safe to show for a rejected OAuth callback.
	ErrInvalidAuthorization = errors.New("oauth authorization was rejected")
	// ErrAuthorizationFailed is safe to show when Wahoo authorization cannot finish.
	ErrAuthorizationFailed = errors.New("oauth authorization could not be completed")
)

// StateStore persists the one-time state and the completed target authorization.
type StateStore interface {
	BeginAuthorization(ctx context.Context, targetID, tailnetUserLogin string, stateDigest []byte, expiresAt time.Time) error
	ConsumeAuthorization(ctx context.Context, tailnetUserLogin string, stateDigest []byte) (string, error)
	AuthorizeTarget(ctx context.Context, targetID, wahooUserID, refreshToken string) error
}

// Wahoo performs the OAuth protocol and learns the authenticated Wahoo user.
type Wahoo interface {
	AuthorizationURL(state string) (string, error)
	ExchangeAuthorizationCode(ctx context.Context, code string) (accessToken, refreshToken string, err error)
	AuthenticatedUser(ctx context.Context, accessToken string) (string, error)
}

// Service coordinates protected Wahoo authorization for configured target slots.
type Service struct {
	stateStore StateStore
	wahoo      Wahoo
}

// New creates an OAuth application service with explicit consumer dependencies.
func New(stateStore StateStore, wahoo Wahoo) (*Service, error) {
	if stateStore == nil || wahoo == nil {
		return nil, errors.New("oauth state store and Wahoo client are required")
	}

	return &Service{stateStore: stateStore, wahoo: wahoo}, nil
}

// Start creates a caller-bound, expiring state and returns Wahoo's authorization URL.
func (s *Service) Start(ctx context.Context, tailnetUserLogin, targetID string) (string, error) {
	state, digest, stateErr := newState()
	if stateErr != nil {
		return "", ErrAuthorizationFailed
	}
	if beginErr := s.stateStore.BeginAuthorization(
		ctx,
		targetID,
		tailnetUserLogin,
		digest,
		time.Now().UTC().Add(stateLifetime),
	); beginErr != nil {
		return "", ErrAuthorizationFailed
	}
	url, err := s.wahoo.AuthorizationURL(state)
	if err != nil {
		return "", ErrAuthorizationFailed
	}

	return url, nil
}

// Complete validates and consumes state, exchanges the code, learns the Wahoo
// user identity, and durably binds the account to the target slot.
func (s *Service) Complete(ctx context.Context, tailnetUserLogin, state, code string) error {
	digest, err := stateDigest(state)
	if err != nil || code == "" {
		return ErrInvalidAuthorization
	}
	targetID, err := s.stateStore.ConsumeAuthorization(ctx, tailnetUserLogin, digest)
	if err != nil {
		return ErrInvalidAuthorization
	}

	accessToken, refreshToken, err := s.wahoo.ExchangeAuthorizationCode(ctx, code)
	if err != nil || accessToken == "" || refreshToken == "" {
		return ErrAuthorizationFailed
	}
	wahooUserID, err := s.wahoo.AuthenticatedUser(ctx, accessToken)
	if err != nil || wahooUserID == "" {
		return ErrAuthorizationFailed
	}
	if err := s.stateStore.AuthorizeTarget(ctx, targetID, wahooUserID, refreshToken); err != nil {
		return ErrAuthorizationFailed
	}

	return nil
}

func newState() (state string, digest []byte, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("reading oauth state randomness: %w", err)
	}
	state = base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256(raw)

	return state, sum[:], nil
}

func stateDigest(state string) ([]byte, error) {
	raw, err := base64.RawURLEncoding.DecodeString(state)
	if err != nil || len(raw) != 32 {
		return nil, errors.New("oauth state is invalid")
	}
	sum := sha256.Sum256(raw)

	return sum[:], nil
}
