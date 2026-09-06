-- The activities this migration deleted cannot be restored: a poll now refuses
-- to store a non-cycling workout, so re-polling the account will not bring them
-- back either. Only the watermark comes back.
DELETE FROM schema_migrations WHERE version = 36;
