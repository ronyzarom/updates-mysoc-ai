-- Add display_name column to instances for friendly names / domain names
ALTER TABLE instances ADD COLUMN IF NOT EXISTS display_name VARCHAR(255);
