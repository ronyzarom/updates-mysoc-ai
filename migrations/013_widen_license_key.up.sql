-- Widen license_key to fit externally issued platform keys
-- (e.g. "mysoc_" + 64 hex chars = 70 chars; original column was VARCHAR(50)).
-- Applied to production manually on 2026-08-19 while onboarding the
-- testing.mysoc.ai platform key.
ALTER TABLE licenses ALTER COLUMN license_key TYPE VARCHAR(128);
