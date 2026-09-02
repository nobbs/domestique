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
	store := openTestStore(t, testKey(1))
	now := time.Unix(1_700_000_000, 0)
	digest := loginDigest(5)
	expiresAt := now.Add(time.Hour)
	require.NoError(t, store.CreateSession(t.Context(), digest, "rider@example.ts.net", "Rider", now, expiresAt), "CreateSession()")

	subject, display, renewedAt, gotExpiresAt, err := store.Session(t.Context(), digest, now)
	require.NoError(t, err, "Session()")
	assert.Equal(t, "rider@example.ts.net", subject, "Session() subject")
	assert.Equal(t, "Rider", display, "Session() display")
	assert.Equal(t, now.Unix(), renewedAt.Unix(), "Session() renewedAt")
	assert.Equal(t, expiresAt.Unix(), gotExpiresAt.Unix(), "Session() expiresAt")

	_, _, _, _, err = store.Session(t.Context(), loginDigest(6), now)
	require.ErrorIs(t, err, ErrSessionNotFound, "Session() unknown digest")
}

func TestStoreReportsExpiredSession(t *testing.T) {
	store := openTestStore(t, testKey(1))
	now := time.Unix(1_700_000_000, 0)
	digest := loginDigest(7)
	require.NoError(t, store.CreateSession(t.Context(), digest, "rider@example.ts.net", "Rider", now, now.Add(time.Minute)), "CreateSession()")

	_, _, _, _, err := store.Session(t.Context(), digest, now.Add(time.Minute))
	require.ErrorIs(t, err, ErrSessionExpired, "Session() at expiry")
}

func TestStoreRenewsSession(t *testing.T) {
	store := openTestStore(t, testKey(1))
	now := time.Unix(1_700_000_000, 0)
	digest := loginDigest(8)
	require.NoError(t, store.CreateSession(t.Context(), digest, "rider@example.ts.net", "Rider", now, now.Add(time.Minute)), "CreateSession()")

	stale := loginDigest(9)
	require.NoError(t, store.CreateSession(t.Context(), stale, "other@example.ts.net", "Other", now, now.Add(20*time.Second)), "CreateSession() stale")

	renewedAt := now.Add(30 * time.Second)
	newExpiresAt := renewedAt.Add(time.Hour)
	require.NoError(t, store.RenewSession(t.Context(), digest, renewedAt, newExpiresAt), "RenewSession()")

	_, _, gotRenewedAt, gotExpiresAt, err := store.Session(t.Context(), digest, renewedAt)
	require.NoError(t, err, "Session() after renewal")
	assert.Equal(t, renewedAt.Unix(), gotRenewedAt.Unix(), "Session() renewedAt after renewal")
	assert.Equal(t, newExpiresAt.Unix(), gotExpiresAt.Unix(), "Session() expiresAt after renewal")

	err = store.RenewSession(t.Context(), loginDigest(10), renewedAt, newExpiresAt)
	require.ErrorIs(t, err, ErrSessionNotFound, "RenewSession() unknown digest")

	_, _, _, _, err = store.Session(t.Context(), stale, renewedAt)
	require.ErrorIs(t, err, ErrSessionNotFound, "Session() a session RenewSession should have pruned")
}

func TestStoreDeletesSession(t *testing.T) {
	store := openTestStore(t, testKey(1))
	now := time.Unix(1_700_000_000, 0)
	digest := loginDigest(11)
	require.NoError(t, store.CreateSession(t.Context(), digest, "rider@example.ts.net", "Rider", now, now.Add(time.Hour)), "CreateSession()")

	require.NoError(t, store.DeleteSession(t.Context(), digest), "DeleteSession()")
	_, _, _, _, err := store.Session(t.Context(), digest, now)
	require.ErrorIs(t, err, ErrSessionNotFound, "Session() after deletion")

	require.NoError(t, store.DeleteSession(t.Context(), digest), "DeleteSession() a second time")
}

// TestStoreRejectsInvalidInput exercises every guard that must fail before a
// method touches the database: a wrong-length digest, a blank nonce or
// verifier, and an expiry that is not strictly after now.
func TestStoreRejectsInvalidInput(t *testing.T) {
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
		"CreateSession short digest":   func() error { return store.CreateSession(t.Context(), short, "subject", "display", now, future) },
		"CreateSession blank subject":  func() error { return store.CreateSession(t.Context(), loginDigest(21), "", "display", now, future) },
		"CreateSession blank display":  func() error { return store.CreateSession(t.Context(), loginDigest(21), "subject", " ", now, future) },
		"CreateSession non-future expiry": func() error {
			return store.CreateSession(t.Context(), loginDigest(21), "subject", "display", now, now)
		},
		"Session short digest":      func() error { _, _, _, _, err := store.Session(t.Context(), short, now); return err },
		"RenewSession short digest": func() error { return store.RenewSession(t.Context(), short, now, future) },
		"RenewSession non-future expiry": func() error {
			return store.RenewSession(t.Context(), loginDigest(22), now, now)
		},
		"DeleteSession short digest": func() error { return store.DeleteSession(t.Context(), short) },
	}
	for name, call := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Error(t, call())
		})
	}
}

func TestStoreCreateSessionPrunesExpiredRows(t *testing.T) {
	store := openTestStore(t, testKey(1))
	now := time.Unix(1_700_000_000, 0)
	stale := loginDigest(12)
	require.NoError(t, store.CreateSession(t.Context(), stale, "rider@example.ts.net", "Rider", now, now.Add(time.Second)), "CreateSession() stale")

	fresh := loginDigest(13)
	require.NoError(t, store.CreateSession(t.Context(), fresh, "rider@example.ts.net", "Rider", now.Add(time.Minute), now.Add(2*time.Minute)), "CreateSession() fresh")

	_, _, _, _, err := store.Session(t.Context(), stale, now.Add(time.Minute))
	require.ErrorIs(t, err, ErrSessionNotFound, "Session() a row CreateSession should have pruned")
}
