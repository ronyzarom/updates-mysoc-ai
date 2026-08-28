-- 015 down: drop the per-customer aggregate table.
DROP INDEX IF EXISTS idx_customer_summaries_name;
DROP INDEX IF EXISTS idx_customer_summaries_exceptions;
DROP TABLE IF EXISTS customer_summaries;
