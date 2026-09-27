// Package sqlite persists Domestique's encrypted, local reconciliation state.
package sqlite

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/nobbs/domestique/internal/sqlite/internal/sqlcgen"
	_ "modernc.org/sqlite" // Pure Go SQLite driver registration.
)

const driverName = "sqlite"

// readConnections is how many reads run at once beside the one writer. WAL lets
// them read committed state while a write is in flight.
const readConnections = 4

// forwardCompatibleMigrations is how far ahead of this binary a state file may
// be and still open: one, so a deploy that failed its health gate can roll back
// onto the previous binary. A release that must stay rollable appends one.
const forwardCompatibleMigrations = 1

// schemaAheadMessage is the stable prefix of that refusal. It is a contract with
// deploy/domestique-deploy.sh, which matches on it.
const schemaAheadMessage = "state schema version is newer than this service"

var (
	// ErrTargetNotFound reports an operation for an unknown configured target slot.
	ErrTargetNotFound = errors.New("target slot not found")
	// ErrRefreshTokenUnavailable reports a target that needs an authorization flow.
	ErrRefreshTokenUnavailable = errors.New("refresh token is unavailable")
	// ErrWahooUserAlreadyAuthorized reports an account already bound to another slot.
	ErrWahooUserAlreadyAuthorized = errors.New("wahoo user is already authorized for another target slot")
	// ErrStateUnreadable reports encrypted state that cannot be authenticated.
	ErrStateUnreadable = errors.New("encrypted state is unreadable")
	// ErrOAuthTransactionNotFound reports an unknown OAuth callback state.
	ErrOAuthTransactionNotFound = errors.New("oauth transaction was not found")
	// ErrOAuthTransactionExpired reports a callback state that exceeded its deadline.
	ErrOAuthTransactionExpired = errors.New("oauth transaction has expired")
	// ErrOAuthTransactionUsed reports a callback state that was already consumed.
	ErrOAuthTransactionUsed = errors.New("oauth transaction was already used")
	// ErrOAuthTransactionIdentityMismatch reports a callback from another identity.
	ErrOAuthTransactionIdentityMismatch = errors.New("oauth transaction identity did not match")
	// ErrLoginNotFound reports an unknown or already-consumed browser sign-in state.
	ErrLoginNotFound = errors.New("login transaction was not found")
	// ErrLoginExpired reports a browser sign-in state that exceeded its deadline.
	ErrLoginExpired = errors.New("login transaction has expired")
	// ErrSessionNotFound reports an unknown web session token.
	ErrSessionNotFound = errors.New("web session was not found")
	// ErrSessionExpired reports a web session token that exceeded its deadline.
	ErrSessionExpired = errors.New("web session has expired")
)

// Store is an SQLite-backed state store whose OAuth tokens use AES-GCM at rest.
type Store struct {
	database *sql.DB
	reader   *sql.DB
	queries  *sqlcgen.Queries
	aead     cipher.AEAD
}

// Open creates or opens a state database, applies pending migrations, and
// configures encrypted token storage. The database path must be absolute.
func Open(ctx context.Context, databasePath string, encryptionKey [32]byte) (*Store, error) {
	if !filepath.IsAbs(databasePath) {
		return nil, errors.New("database path must be absolute")
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		return nil, fmt.Errorf("creating state directory: %w", err)
	}

	block, err := aes.NewCipher(encryptionKey[:])
	if err != nil {
		return nil, fmt.Errorf("creating state cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating state cipher: %w", err)
	}

	database, err := openDatabase(databasePath)
	if err != nil {
		return nil, fmt.Errorf("opening state database: %w", err)
	}
	store := &Store{database: database, aead: aead}
	if err := store.configure(ctx); err != nil {
		closeDatabase(database)
		return nil, err
	}
	if err := store.migrate(ctx, databasePath); err != nil {
		closeDatabase(database)
		return nil, err
	}
	if err := os.Chmod(databasePath, 0o600); err != nil {
		closeDatabase(database)
		return nil, fmt.Errorf("protecting state database: %w", err)
	}
	// sql.Open fails only for an unregistered driver: modernc parses the name on
	// first connection, which the writer has already made with this DSN.
	reader, _ := sql.Open(driverName, databaseDSN(databasePath)+"&_pragma=query_only(1)") //nolint:errcheck // Cannot fail.
	reader.SetMaxOpenConns(readConnections)
	reader.SetMaxIdleConns(readConnections)
	store.reader = reader
	store.queries = sqlcgen.New(routed{writer: database, reader: reader})

	return store, nil
}

// Close releases the database connections.
func (s *Store) Close() error {
	if err := errors.Join(s.reader.Close(), s.database.Close()); err != nil {
		return fmt.Errorf("closing state database: %w", err)
	}

	return nil
}

// routed sends a statement that only reads to the read pool, so it never waits
// for the writer's single connection, and every other statement to the writer.
type routed struct{ writer, reader *sql.DB }

func (r routed) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return r.writer.ExecContext(ctx, query, args...) //nolint:wrapcheck // sqlc's callers wrap.
}

func (r routed) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return r.writer.PrepareContext(ctx, query) //nolint:wrapcheck // sqlc's callers wrap.
}

func (r routed) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return r.pool(query).QueryContext(ctx, query, args...) //nolint:wrapcheck // sqlc's callers wrap.
}

func (r routed) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return r.pool(query).QueryRowContext(ctx, query, args...)
}

// pool reads past sqlc's leading "-- name:" comment lines. A statement that
// writes and returns rows (INSERT ... RETURNING) is not a SELECT, so it stays
// on the writer.
func (r routed) pool(query string) *sql.DB {
	for {
		query = strings.TrimSpace(query)
		if !strings.HasPrefix(query, "--") {
			break
		}
		_, query, _ = strings.Cut(query, "\n")
	}
	if len(query) >= 6 && strings.EqualFold(query[:6], "SELECT") {
		return r.reader
	}

	return r.writer
}

func (s *Store) configure(ctx context.Context) error {
	if err := s.database.PingContext(ctx); err != nil {
		return fmt.Errorf("opening state database: %w", err)
	}
	return nil
}

func openDatabase(databasePath string) (*sql.DB, error) {
	database, err := sql.Open(driverName, databaseDSN(databasePath))
	if err != nil {
		return nil, fmt.Errorf("opening state database: %w", err)
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	return database, nil
}

func databaseDSN(databasePath string) string {
	if filepath.IsAbs(databasePath) {
		databasePath = (&url.URL{Scheme: "file", Path: databasePath}).String()
	}
	separator := "?"
	if strings.Contains(databasePath, "?") {
		separator = "&"
	}
	return databasePath + separator + "_foreign_keys=on&_busy_timeout=5000&_journal_mode=WAL"
}

//nolint:errcheck // A cleanup error cannot replace the already-returned state error.
func closeDatabase(database *sql.DB) {
	_ = database.Close()
}

//nolint:errcheck // A query cleanup error is superseded by the query result.
func closeRows(rows *sql.Rows) {
	_ = rows.Close()
}

//nolint:errcheck // Rollback after a committed transaction is expected to return sql.ErrTxDone.
func rollback(transaction *sql.Tx) {
	_ = transaction.Rollback()
}

//nolint:errcheck // A statement cleanup error is superseded by the write's own result.
func closeStatement(statement *sql.Stmt) {
	_ = statement.Close()
}

// withTx runs write inside one transaction, committing only when it succeeds.
func (s *Store) withTx(ctx context.Context, what string, write func(*sqlcgen.Queries) error) error {
	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting the %s write: %w", what, err)
	}
	defer rollback(transaction)
	if err := write(s.queries.WithTx(transaction)); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("committing the %s: %w", what, err)
	}
	return nil
}

func boolInteger(value bool) int64 {
	if value {
		return 1
	}
	return 0
}
