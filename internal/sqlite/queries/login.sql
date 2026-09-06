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
-- NULL with no default, so dropping it needs a rebuild. nickname is nullable:
-- a token without the claim stores nothing rather than a guess.
INSERT INTO web_sessions (token_digest, subject, display, nickname, admin, created_at_unix, renewed_at_unix, expires_at_unix)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetWebSession :one
SELECT subject, display, COALESCE(nickname, '') AS nickname, admin, expires_at_unix FROM web_sessions WHERE token_digest = ?;

-- name: DeleteWebSession :exec
DELETE FROM web_sessions WHERE token_digest = ?;

-- name: ListLatestSessionNicknames :many
-- One row per subject that ever signed in with a nickname: the one from its
-- newest such session, ties broken by rowid so exactly one row answers.
-- Keyed by subject throughout: this never looks a rider up by nickname.
SELECT subject, nickname
FROM web_sessions AS latest
WHERE rowid = (
  SELECT rowid FROM web_sessions AS candidate
  WHERE candidate.subject = latest.subject
    AND candidate.nickname IS NOT NULL AND candidate.nickname != ''
  ORDER BY candidate.created_at_unix DESC, candidate.rowid DESC
  LIMIT 1
);
