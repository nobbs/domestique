package oauth

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestServiceCompletesBoundAuthorization(t *testing.T) {
	store := &fakeStateStore{}
	wahoo := &fakeWahoo{accessToken: "access-token", refreshToken: "refresh-token", userID: "wahoo-user"}
	service, err := New(store, wahoo)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	url, err := service.Start(t.Context(), "rider@example.ts.net", "rider-a")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	state := strings.TrimPrefix(url, "https://wahoo.example.test/authorize?state=")
	if state == url {
		t.Fatalf("Start() URL = %q, want an authorization state", url)
	}
	if completeErr := service.Complete(t.Context(), "rider@example.ts.net", state, "authorization-code"); completeErr != nil {
		t.Fatalf("Complete() error = %v", completeErr)
	}
	if got, want := store.authorizedTarget, "rider-a"; got != want {
		t.Errorf("authorized target = %q, want %q", got, want)
	}
	if got, want := store.authorizedUser, "wahoo-user"; got != want {
		t.Errorf("authorized user = %q, want %q", got, want)
	}
	if got, want := store.refreshToken, "refresh-token"; got != want {
		t.Errorf("refresh token = %q, want %q", got, want)
	}
}

func TestServiceRejectsInvalidOrReusedAuthorization(t *testing.T) {
	store := &fakeStateStore{}
	service, err := New(store, &fakeWahoo{accessToken: "access", refreshToken: "refresh", userID: "wahoo-user"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if completeErr := service.Complete(t.Context(), "rider@example.ts.net", "not-base64", "code"); !errors.Is(completeErr, ErrInvalidAuthorization) {
		t.Fatalf("Complete() error = %v, want ErrInvalidAuthorization", completeErr)
	}

	url, err := service.Start(t.Context(), "rider@example.ts.net", "rider-a")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	state := strings.TrimPrefix(url, "https://wahoo.example.test/authorize?state=")
	if err := service.Complete(t.Context(), "other@example.ts.net", state, "code"); !errors.Is(err, ErrInvalidAuthorization) {
		t.Fatalf("Complete() with different caller error = %v, want ErrInvalidAuthorization", err)
	}
	if err := service.Complete(t.Context(), "rider@example.ts.net", state, "code"); err != nil {
		t.Fatalf("Complete() with original caller error = %v", err)
	}
	if err := service.Complete(t.Context(), "rider@example.ts.net", state, "code"); !errors.Is(err, ErrInvalidAuthorization) {
		t.Fatalf("Complete() after state reuse error = %v, want ErrInvalidAuthorization", err)
	}
}

func TestServiceHidesUpstreamFailure(t *testing.T) {
	store := &fakeStateStore{}
	service, err := New(store, &fakeWahoo{exchangeErr: errors.New("code=private-code")})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	url, err := service.Start(t.Context(), "rider@example.ts.net", "rider-a")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	state := strings.TrimPrefix(url, "https://wahoo.example.test/authorize?state=")
	err = service.Complete(t.Context(), "rider@example.ts.net", state, "code")
	if !errors.Is(err, ErrAuthorizationFailed) {
		t.Fatalf("Complete() error = %v, want ErrAuthorizationFailed", err)
	}
	if strings.Contains(err.Error(), "private-code") {
		t.Errorf("Complete() exposed upstream error %q", err)
	}
}

type fakeStateStore struct {
	expiresAt        time.Time
	authorizedTarget string
	authorizedUser   string
	refreshToken     string
	targetID         string
	caller           string
	digest           []byte
	used             bool
}

func (s *fakeStateStore) BeginAuthorization(
	_ context.Context,
	targetID, tailnetUserLogin string,
	stateDigest []byte,
	expiresAt time.Time,
) error {
	s.digest = append([]byte(nil), stateDigest...)
	s.targetID = targetID
	s.caller = tailnetUserLogin
	s.expiresAt = expiresAt

	return nil
}

func (s *fakeStateStore) ConsumeAuthorization(_ context.Context, tailnetUserLogin string, stateDigest []byte) (string, error) {
	if s.used || !s.expiresAt.After(time.Now()) {
		return "", errors.New("state was unavailable")
	}
	if tailnetUserLogin != s.caller || !bytes.Equal(stateDigest, s.digest) {
		return "", errors.New("state did not match")
	}
	s.used = true

	return s.targetID, nil
}

func (s *fakeStateStore) AuthorizeTarget(_ context.Context, targetID, wahooUserID, refreshToken string) error {
	s.authorizedTarget = targetID
	s.authorizedUser = wahooUserID
	s.refreshToken = refreshToken

	return nil
}

type fakeWahoo struct {
	exchangeErr  error
	accessToken  string
	refreshToken string
	userID       string
}

func (w *fakeWahoo) AuthorizationURL(state string) (string, error) {
	return "https://wahoo.example.test/authorize?state=" + state, nil
}

func (w *fakeWahoo) ExchangeAuthorizationCode(_ context.Context, _ string) (accessToken, refreshToken string, err error) {
	if w.exchangeErr != nil {
		return "", "", w.exchangeErr
	}

	return w.accessToken, w.refreshToken, nil
}

func (w *fakeWahoo) AuthenticatedUser(_ context.Context, _ string) (string, error) {
	return w.userID, nil
}
