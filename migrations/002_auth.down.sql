-- Rollback authentication schema

DROP TRIGGER IF EXISTS update_users_updated_at ON users;
DROP TABLE IF EXISTS auth_audit_log;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
