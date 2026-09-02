DELETE FROM schema_migrations WHERE version = 28;
DROP INDEX web_sessions_expiry_index;
DROP TABLE web_sessions;
DROP TABLE login_transactions;
