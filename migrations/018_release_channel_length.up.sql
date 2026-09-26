-- Product qualification channels may include a role and date. Preserve exact
-- names without changing existing releases or their target groups.
ALTER TABLE releases ALTER COLUMN channel TYPE VARCHAR(64);
