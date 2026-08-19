-- Best-effort rollback: only safe if no keys longer than 50 chars exist.
ALTER TABLE licenses ALTER COLUMN license_key TYPE VARCHAR(50);
