-- Refuse rollback rather than truncate an existing isolated channel identity.
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM releases WHERE length(channel) > 20) THEN
        RAISE EXCEPTION 'release channels longer than 20 characters prevent rollback';
    END IF;
END $$;
ALTER TABLE releases ALTER COLUMN channel TYPE VARCHAR(20);
