package sqlite

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loginDigest(value byte) []byte {
	return bytes.Repeat([]byte{value}, 32)
}

func TestStoreConsumesLoginOnce(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	now := time.Unix(1_700_000_000, 0)
	digest := loginDigest(1)
	require.NoError(t, store.BeginLogin(t.Context(), digest, "nonce-a", "verifier-a", now, now.Add(time.Minute)), "BeginLogin()")

	nonce, codeVerifier, err := store.ConsumeLogin(t.Context(), digest, now)
	require.NoError(t, err, "ConsumeLogin()")
	assert.Equal(t, "nonce-a", nonce, "ConsumeLogin() nonce")
	assert.Equal(t, "verifier-a", codeVerifier, "ConsumeLogin() code verifier")

	_, _, err = store.ConsumeLogin(t.Context(), digest, now)
	require.ErrorIs(t, err, ErrLoginNotFound, "ConsumeLogin() after consumption")
}

func TestStoreRejectsExpiredLoginConsumption(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	now := time.Unix(1_700_000_000, 0)
	digest := loginDigest(2)
	require.NoError(t, store.BeginLogin(t.Context(), digest, "nonce", "verifier", now, now.Add(time.Minute)), "BeginLogin()")

	_, _, err := store.ConsumeLogin(t.Context(), digest, now.Add(2*time.Minute))
	require.ErrorIs(t, err, ErrLoginExpired, "ConsumeLogin() past expiry")

	_, _, err = store.ConsumeLogin(t.Context(), digest, now)
	require.ErrorIs(t, err, ErrLoginNotFound, "ConsumeLogin() after expiry consumed the row")
}

func TestStoreBeginLoginPrunesExpiredRows(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	now := time.Unix(1_700_000_000, 0)
	stale := loginDigest(3)
	require.NoError(t, store.BeginLogin(t.Context(), stale, "nonce", "verifier", now, now.Add(time.Second)), "BeginLogin() stale")

	fresh := loginDigest(4)
	require.NoError(t, store.BeginLogin(t.Context(), fresh, "nonce", "verifier", now.Add(time.Minute), now.Add(2*time.Minute)), "BeginLogin() fresh")

	_, _, err := store.ConsumeLogin(t.Context(), stale, now.Add(time.Minute))
	require.ErrorIs(t, err, ErrLoginNotFound, "ConsumeLogin() a row BeginLogin should have pruned")
}

func TestStoreBeginLoginCapsTransactionCount(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	now := time.Unix(1_700_000_000, 0)

	const attempts = 70
	for index := range attempts {
		digest := loginDigest(byte(index))
		expiresAt := now.Add(time.Duration(index+1) * time.Minute)
		require.NoError(t, store.BeginLogin(t.Context(), digest, "nonce", "verifier", now, expiresAt), "BeginLogin()")
	}

	var remaining int
	require.NoError(t, store.database.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM login_transactions").Scan(&remaining), "counting login transactions")
	assert.LessOrEqual(t, remaining, maximumLoginTransactions, "login transactions retained beyond the cap")

	newest := loginDigest(byte(attempts - 1))
	_, _, err := store.ConsumeLogin(t.Context(), newest, now)
	require.NoError(t, err, "ConsumeLogin() the newest attempt must survive eviction")

	oldest := loginDigest(0)
	_, _, err = store.ConsumeLogin(t.Context(), oldest, now)
	require.ErrorIs(t, err, ErrLoginNotFound, "ConsumeLogin() the oldest attempt must be evicted")
}

func TestStoreBeginLoginKeepsNewestWhenExpiriesTie(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	now := time.Unix(1_700_000_000, 0)
	// Expiry is stored with second precision, so a burst within one second ties
	// on expires_at_unix and eviction has to fall back on insertion order.
	expiresAt := now.Add(time.Minute)

	const attempts = 70
	for index := range attempts {
		require.NoError(t, store.BeginLogin(t.Context(), loginDigest(byte(index)), "nonce", "verifier", now, expiresAt), "BeginLogin()")
	}

	for index := attempts - maximumLoginTransactions; index < attempts; index++ {
		_, _, err := store.ConsumeLogin(t.Context(), loginDigest(byte(index)), now)
		require.NoErrorf(t, err, "ConsumeLogin() attempt %d must survive eviction", index)
	}

	_, _, err := store.ConsumeLogin(t.Context(), loginDigest(0), now)
	require.ErrorIs(t, err, ErrLoginNotFound, "ConsumeLogin() the oldest tied attempt must be evicted")
}

func TestStoreRoundTripsSession(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	now := time.Unix(1_700_000_000, 0)
	digest := loginDigest(5)
	expiresAt := now.Add(time.Hour)
	require.NoError(t, store.CreateSession(t.Context(), digest, "rider@example.ts.net", "Rider", "riderly", true, now, expiresAt), "CreateSession()")

	subject, display, nickname, admin, err := store.Session(t.Context(), digest, now)
	require.NoError(t, err, "Session()")
	assert.Equal(t, "rider@example.ts.net", subject, "Session() subject")
	assert.Equal(t, "Rider", display, "Session() display")
	assert.Equal(t, "riderly", nickname, "Session() nickname")
	assert.True(t, admin, "Session() admin")

	_, _, _, _, err = store.Session(t.Context(), loginDigest(6), now)
	require.ErrorIs(t, err, ErrSessionNotFound, "Session() unknown digest")
}

// A session created without a nickname claim reports it empty rather than
// falling back to display or subject.
func TestStoreRoundTripsSessionWithoutNickname(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	now := time.Unix(1_700_000_000, 0)
	digest := loginDigest(8)
	require.NoError(t, store.CreateSession(t.Context(), digest, "rider@example.ts.net", "Rider", "", false, now, now.Add(time.Hour)), "CreateSession()")

	_, _, nickname, _, err := store.Session(t.Context(), digest, now)
	require.NoError(t, err, "Session()")
	assert.Empty(t, nickname, "Session() nickname")
}

func TestStoreReportsExpiredSession(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	now := time.Unix(1_700_000_000, 0)
	digest := loginDigest(7)
	require.NoError(t, store.CreateSession(t.Context(), digest, "rider@example.ts.net", "Rider", "", false, now, now.Add(time.Minute)), "CreateSession()")

	_, _, _, _, err := store.Session(t.Context(), digest, now.Add(time.Minute))
	require.ErrorIs(t, err, ErrSessionExpired, "Session() at expiry")
}

func TestStoreDeletesSession(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	now := time.Unix(1_700_000_000, 0)
	digest := loginDigest(11)
	require.NoError(t, store.CreateSession(t.Context(), digest, "rider@example.ts.net", "Rider", "", false, now, now.Add(time.Hour)), "CreateSession()")

	require.NoError(t, store.DeleteSession(t.Context(), digest), "DeleteSession()")
	_, _, _, _, err := store.Session(t.Context(), digest, now)
	require.ErrorIs(t, err, ErrSessionNotFound, "Session() after deletion")

	require.NoError(t, store.DeleteSession(t.Context(), digest), "DeleteSession() a second time")
}

// LatestSessionNicknames is keyed by subject, one entry per subject that has
// ever signed in with a nickname; a subject that never has is simply absent,
// and a later session's nickname is what a stale earlier one does not shadow.
// A nickname of only whitespace is no nickname, and one with padding loses it.
func TestStoreTrimsTheNickname(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	now := time.Unix(1_700_000_000, 0)
	require.NoError(t, store.CreateSession(t.Context(), loginDigest(50), "github|1", "One", "  padded  ", false, now, now.Add(time.Hour)), "CreateSession()")
	require.NoError(t, store.CreateSession(t.Context(), loginDigest(51), "github|2", "Two", "   ", false, now, now.Add(time.Hour)), "CreateSession()")

	nicknames, err := store.LatestSessionNicknames(t.Context())
	require.NoError(t, err, "LatestSessionNicknames()")
	assert.Equal(t, map[string]string{"github|1": "padded"}, nicknames)
}

func TestStoreLatestSessionNicknames(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	now := time.Unix(1_700_000_000, 0)
	require.NoError(t, store.CreateSession(t.Context(), loginDigest(40), "rider-a", "Rider A", "Ry", false, now, now.Add(time.Hour)), "CreateSession() rider-a")
	require.NoError(t, store.CreateSession(t.Context(), loginDigest(41), "rider-b", "Rider B", "", false, now, now.Add(time.Hour)), "CreateSession() rider-b without nickname")
	require.NoError(t, store.CreateSession(t.Context(), loginDigest(42), "rider-a", "Rider A", "Ryan", false, now.Add(time.Minute), now.Add(time.Hour)), "CreateSession() rider-a again")

	nicknames, err := store.LatestSessionNicknames(t.Context())
	require.NoError(t, err, "LatestSessionNicknames()")
	assert.Equal(t, map[string]string{"rider-a": "Ryan"}, nicknames)
}

// TestStoreRejectsInvalidInput exercises every guard that must fail before a
// method touches the database: a wrong-length digest, a blank nonce or
// verifier, and an expiry that is not strictly after now.
func TestStoreRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	now := time.Unix(1_700_000_000, 0)
	future := now.Add(time.Minute)
	short := []byte{1, 2, 3}

	tests := map[string]func() error{
		"BeginLogin short digest":      func() error { return store.BeginLogin(t.Context(), short, "nonce", "verifier", now, future) },
		"BeginLogin blank nonce":       func() error { return store.BeginLogin(t.Context(), loginDigest(20), "", "verifier", now, future) },
		"BeginLogin blank verifier":    func() error { return store.BeginLogin(t.Context(), loginDigest(20), "nonce", "", now, future) },
		"BeginLogin non-future expiry": func() error { return store.BeginLogin(t.Context(), loginDigest(20), "nonce", "verifier", now, now) },
		"ConsumeLogin short digest":    func() error { _, _, err := store.ConsumeLogin(t.Context(), short, now); return err },
		"CreateSession short digest": func() error {
			return store.CreateSession(t.Context(), short, "subject", "display", "", false, now, future)
		},
		"CreateSession blank subject": func() error {
			return store.CreateSession(t.Context(), loginDigest(21), "", "display", "", false, now, future)
		},
		"CreateSession blank display": func() error {
			return store.CreateSession(t.Context(), loginDigest(21), "subject", " ", "", false, now, future)
		},
		"CreateSession non-future expiry": func() error {
			return store.CreateSession(t.Context(), loginDigest(21), "subject", "display", "", false, now, now)
		},
		"Session short digest":       func() error { _, _, _, _, err := store.Session(t.Context(), short, now); return err },
		"DeleteSession short digest": func() error { return store.DeleteSession(t.Context(), short) },
	}
	for name, call := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Error(t, call())
		})
	}
}

func TestStoreCreateSessionPrunesExpiredRows(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	now := time.Unix(1_700_000_000, 0)
	stale := loginDigest(12)
	require.NoError(t, store.CreateSession(t.Context(), stale, "rider@example.ts.net", "Rider", "", false, now, now.Add(time.Second)), "CreateSession() stale")

	fresh := loginDigest(13)
	require.NoError(t, store.CreateSession(t.Context(), fresh, "rider@example.ts.net", "Rider", "", false, now.Add(time.Minute), now.Add(2*time.Minute)), "CreateSession() fresh")

	_, _, _, _, err := store.Session(t.Context(), stale, now.Add(time.Minute))
	require.ErrorIs(t, err, ErrSessionNotFound, "Session() a row CreateSession should have pruned")
}

func TestStoreReportsLoginStorageFailures(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_700_000_000, 0)
	future := now.Add(time.Minute)

	tests := map[string]struct {
		call  func(*Store) error
		table string
	}{
		"BeginLogin": {table: "login_transactions", call: func(store *Store) error {
			return store.BeginLogin(t.Context(), loginDigest(30), "nonce", "verifier", now, future)
		}},
		"ConsumeLogin": {table: "login_transactions", call: func(store *Store) error {
			_, _, err := store.ConsumeLogin(t.Context(), loginDigest(30), now)
			return err
		}},
		"CreateSession": {table: "web_sessions", call: func(store *Store) error {
			return store.CreateSession(t.Context(), loginDigest(31), "subject", "display", "", false, now, future)
		}},
		"Session": {table: "web_sessions", call: func(store *Store) error {
			_, _, _, _, err := store.Session(t.Context(), loginDigest(31), now)
			return err
		}},
		"DeleteSession": {table: "web_sessions", call: func(store *Store) error {
			return store.DeleteSession(t.Context(), loginDigest(31))
		}},
		"LatestSessionNicknames": {table: "web_sessions", call: func(store *Store) error {
			_, err := store.LatestSessionNicknames(t.Context())
			return err
		}},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			store := openTestStore(t, testKey(1))
			_, err := store.database.ExecContext(t.Context(), "DROP TABLE "+test.table)
			require.NoError(t, err, "dropping %s", test.table)

			assert.Error(t, test.call(store))
		})
	}
}

func TestStoreRejectsDuplicateDigests(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	now := time.Unix(1_700_000_000, 0)
	future := now.Add(time.Minute)

	state := loginDigest(32)
	require.NoError(t, store.BeginLogin(t.Context(), state, "nonce", "verifier", now, future), "BeginLogin()")
	require.Error(t, store.BeginLogin(t.Context(), state, "nonce", "verifier", now, future), "BeginLogin() reusing a state digest")

	token := loginDigest(33)
	require.NoError(t, store.CreateSession(t.Context(), token, "subject", "display", "", false, now, future), "CreateSession()")
	assert.Error(t, store.CreateSession(t.Context(), token, "subject", "display", "", false, now, future), "CreateSession() reusing a token digest")
}
