// Package oauth owns Wahoo target authorization use cases.
package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
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
	BeginAuthorization(ctx context.Context, targetID, callerLogin string, stateDigest []byte, expiresAt time.Time) error
	ConsumeAuthorization(ctx context.Context, callerLogin string, stateDigest []byte) (string, error)
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
func (s *Service) Start(ctx context.Context, callerLogin, targetID string) (string, error) {
	state, digest, stateErr := newState()
	if stateErr != nil {
		return "", ErrAuthorizationFailed
	}
	if beginErr := s.stateStore.BeginAuthorization(
		ctx,
		targetID,
		callerLogin,
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
//
// Each step names itself when it refuses. The caller answers all four the same
// way, so the step is the only thing that tells a spent state apart from a
// rejected client secret or an account already bound to another slot.
func (s *Service) Complete(ctx context.Context, callerLogin, state, code string) error {
	if state == "" {
		slog.Warn("wahoo authorization refused", "reason", "callback_state_missing")

		return ErrInvalidAuthorization
	}
	digest, err := stateDigest(state)
	if err != nil {
		slog.Warn("wahoo authorization refused", "reason", "callback_state_unusable")

		return ErrInvalidAuthorization
	}
	// Named apart from the state, and matching the HTTP handler's own reason for
	// it: the handler refuses an empty code before this, but another entry point
	// need not, and one reason for two causes is what this change exists to undo.
	if code == "" {
		slog.Warn("wahoo authorization refused", "reason", "callback_code_missing")

		return ErrInvalidAuthorization
	}
	targetID, err := s.stateStore.ConsumeAuthorization(ctx, callerLogin, digest)
	if err != nil {
		// This store's own sentinels — expired, spent, unknown, wrong caller —
		// name a state of the flow and carry nothing a rider entered.
		slog.Warn("wahoo authorization refused", "reason", "state_not_consumed", "error", err)

		return ErrInvalidAuthorization
	}

	accessToken, refreshToken, err := s.wahoo.ExchangeAuthorizationCode(ctx, code)
	if err != nil || accessToken == "" || refreshToken == "" {
		// The step alone: an x/oauth2 retrieval error carries the token
		// endpoint's raw response body.
		slog.Warn("wahoo authorization refused", "reason", "code_exchange_failed")

		return ErrAuthorizationFailed
	}
	wahooUserID, err := s.wahoo.AuthenticatedUser(ctx, accessToken)
	if err != nil || wahooUserID == "" {
		slog.Warn("wahoo authorization refused", "reason", "wahoo_user_unknown")

		return ErrAuthorizationFailed
	}
	if err := s.stateStore.AuthorizeTarget(ctx, targetID, wahooUserID, refreshToken); err != nil {
		slog.Warn("wahoo authorization refused", "reason", "target_not_bound", "error", err)

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
