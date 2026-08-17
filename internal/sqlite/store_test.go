package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestStoreAuthorizesAndEncryptsRefreshToken(t *testing.T) {
	store := openTestStore(t, testKey(1))
	if err := store.EnsureTargets(t.Context(), []string{"rider-a", "rider-b"}); err != nil {
		t.Fatalf("EnsureTargets() error = %v", err)
	}
	if err := store.AuthorizeTarget(t.Context(), "rider-a", "wahoo-user", "refresh-token"); err != nil {
		t.Fatalf("AuthorizeTarget() error = %v", err)
	}

	target, err := store.Target(t.Context(), "rider-a")
	if err != nil {
		t.Fatalf("Target() error = %v", err)
	}
	if got, want := target.AuthorizationState, AuthorizationAuthorized; got != want {
		t.Errorf("Target().AuthorizationState = %q, want %q", got, want)
	}
	if got, want := target.WahooUserID, "wahoo-user"; got != want {
		t.Errorf("Target().WahooUserID = %q, want %q", got, want)
	}

	var encrypted []byte
	if queryErr := store.database.QueryRowContext(t.Context(), "SELECT refresh_token FROM targets WHERE slot = ?", "rider-a").Scan(&encrypted); queryErr != nil {
		t.Fatalf("query encrypted token: %v", queryErr)
	}
	if bytes.Contains(encrypted, []byte("refresh-token")) {
		t.Error("database stores refresh token in plaintext")
	}

	got, err := store.RefreshToken(t.Context(), "rider-a")
	if err != nil {
		t.Fatalf("RefreshToken() error = %v", err)
	}
	if want := "refresh-token"; got != want {
		t.Errorf("RefreshToken() = %q, want %q", got, want)
	}
}

func TestStoreRejectsDuplicateWahooUser(t *testing.T) {
	store := openTestStore(t, testKey(1))
	if err := store.EnsureTargets(t.Context(), []string{"rider-a", "rider-b"}); err != nil {
		t.Fatalf("EnsureTargets() error = %v", err)
	}
	if err := store.AuthorizeTarget(t.Context(), "rider-a", "wahoo-user", "token-a"); err != nil {
		t.Fatalf("AuthorizeTarget(rider-a) error = %v", err)
	}
	if err := store.AuthorizeTarget(t.Context(), "rider-b", "wahoo-user", "token-b"); !errors.Is(err, ErrWahooUserAlreadyAuthorized) {
		t.Errorf("AuthorizeTarget(rider-b) error = %v, want %v", err, ErrWahooUserAlreadyAuthorized)
	}
}

func TestStoreBindsTokenToTarget(t *testing.T) {
	store := openTestStore(t, testKey(1))
	if err := store.EnsureTargets(t.Context(), []string{"rider-a", "rider-b"}); err != nil {
		t.Fatalf("EnsureTargets() error = %v", err)
	}
	if err := store.AuthorizeTarget(t.Context(), "rider-a", "wahoo-user-a", "token-a"); err != nil {
		t.Fatalf("AuthorizeTarget() error = %v", err)
	}

	var encrypted []byte
	if err := store.database.QueryRowContext(t.Context(), "SELECT refresh_token FROM targets WHERE slot = ?", "rider-a").Scan(&encrypted); err != nil {
		t.Fatalf("query encrypted token: %v", err)
	}
	if _, err := store.database.ExecContext(t.Context(), "UPDATE targets SET refresh_token = ? WHERE slot = ?", encrypted, "rider-b"); err != nil {
		t.Fatalf("copy encrypted token: %v", err)
	}

	_, err := store.RefreshToken(t.Context(), "rider-b")
	if !errors.Is(err, ErrStateUnreadable) {
		t.Errorf("RefreshToken() error = %v, want %v", err, ErrStateUnreadable)
	}
}

func TestStoreRejectsDifferentEncryptionKey(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "state.db")
	store, openErr := Open(t.Context(), databasePath, testKey(1))
	if openErr != nil {
		t.Fatalf("Open() error = %v", openErr)
	}
	if ensureErr := store.EnsureTargets(t.Context(), []string{"rider-a"}); ensureErr != nil {
		t.Fatalf("EnsureTargets() error = %v", ensureErr)
	}
	if authorizeErr := store.AuthorizeTarget(t.Context(), "rider-a", "wahoo-user", "refresh-token"); authorizeErr != nil {
		t.Fatalf("AuthorizeTarget() error = %v", authorizeErr)
	}
	if closeErr := store.Close(); closeErr != nil {
		t.Fatalf("Close() error = %v", closeErr)
	}

	reopened, err := Open(t.Context(), databasePath, testKey(2))
	if err != nil {
		t.Fatalf("Open() with different key error = %v", err)
	}
	t.Cleanup(func() {
		if err := reopened.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	if _, err := reopened.RefreshToken(t.Context(), "rider-a"); !errors.Is(err, ErrStateUnreadable) {
		t.Errorf("RefreshToken() error = %v, want %v", err, ErrStateUnreadable)
	}
}

func TestStoreMarksTargetForReauthorization(t *testing.T) {
	store := openTestStore(t, testKey(1))
	if err := store.EnsureTargets(t.Context(), []string{"rider-a"}); err != nil {
		t.Fatalf("EnsureTargets() error = %v", err)
	}
	if err := store.AuthorizeTarget(t.Context(), "rider-a", "wahoo-user", "refresh-token"); err != nil {
		t.Fatalf("AuthorizeTarget() error = %v", err)
	}
	if err := store.MarkNeedsReauthorization(t.Context(), "rider-a"); err != nil {
		t.Fatalf("MarkNeedsReauthorization() error = %v", err)
	}

	target, err := store.Target(t.Context(), "rider-a")
	if err != nil {
		t.Fatalf("Target() error = %v", err)
	}
	if got, want := target.AuthorizationState, AuthorizationNeedsReauthorization; got != want {
		t.Errorf("Target().AuthorizationState = %q, want %q", got, want)
	}
	if _, err := store.RefreshToken(t.Context(), "rider-a"); !errors.Is(err, ErrRefreshTokenUnavailable) {
		t.Errorf("RefreshToken() error = %v, want %v", err, ErrRefreshTokenUnavailable)
	}
}

func TestStoreMigrationsAreIdempotent(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "state.db")
	first, firstOpenErr := Open(t.Context(), databasePath, testKey(1))
	if firstOpenErr != nil {
		t.Fatalf("first Open() error = %v", firstOpenErr)
	}
	if firstCloseErr := first.Close(); firstCloseErr != nil {
		t.Fatalf("first Close() error = %v", firstCloseErr)
	}

	second, secondOpenErr := Open(t.Context(), databasePath, testKey(1))
	if secondOpenErr != nil {
		t.Fatalf("second Open() error = %v", secondOpenErr)
	}
	t.Cleanup(func() {
		if err := second.Close(); err != nil {
			t.Errorf("second Close() error = %v", err)
		}
	})

	var version int
	if err := second.database.QueryRowContext(t.Context(), "SELECT MAX(version) FROM schema_migrations").Scan(&version); err != nil {
		t.Fatalf("query migration version: %v", err)
	}
	if got, want := version, len(schemaMigrations()); got != want {
		t.Errorf("schema version = %d, want %d", got, want)
	}
}

func openTestStore(t *testing.T, key [32]byte) *Store {
	t.Helper()

	store, err := Open(context.Background(), filepath.Join(t.TempDir(), "state.db"), key)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil && !errors.Is(err, sql.ErrConnDone) {
			t.Errorf("Close() error = %v", err)
		}
	})

	return store
}

func testKey(value byte) [32]byte {
	var key [32]byte
	for index := range key {
		key[index] = value
	}

	return key
}
