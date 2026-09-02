-- name: DeleteExpiredLoginTransactions :exec
DELETE FROM login_transactions WHERE expires_at_unix <= ?;

-- name: CapLoginTransactions :exec
DELETE FROM login_transactions
WHERE state_digest NOT IN (
  SELECT state_digest FROM login_transactions ORDER BY expires_at_unix DESC, rowid DESC LIMIT ?
);

-- name: InsertLoginTransaction :exec
INSERT INTO login_transactions (state_digest, nonce, code_verifier, expires_at_unix)
VALUES (?, ?, ?, ?);

-- name: GetLoginTransaction :one
SELECT nonce, code_verifier, expires_at_unix FROM login_transactions WHERE state_digest = ?;

-- name: DeleteLoginTransaction :exec
DELETE FROM login_transactions WHERE state_digest = ?;

-- name: DeleteExpiredWebSessions :exec
DELETE FROM web_sessions WHERE expires_at_unix <= ?;

-- name: InsertWebSession :exec
-- renewed_at_unix is unused, superseded by the fixed 24-hour lifetime; NOT
-- NULL with no default, so dropping it needs a rebuild.
INSERT INTO web_sessions (token_digest, subject, display, admin, created_at_unix, renewed_at_unix, expires_at_unix)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetWebSession :one
SELECT subject, display, admin, expires_at_unix FROM web_sessions WHERE token_digest = ?;

-- name: DeleteWebSession :exec
DELETE FROM web_sessions WHERE token_digest = ?;
