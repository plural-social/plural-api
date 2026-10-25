DROP TABLE IF EXISTS report;
DROP TABLE IF EXISTS block;
ALTER TABLE account DROP CONSTRAINT account_role_check, DROP COLUMN suspended_at, DROP COLUMN role;
