-- MySoc Updates Platform - Rollback Initial Schema
-- Run with: psql -d mysoc_updates -f migrations/001_initial.down.sql

-- Drop triggers
DROP TRIGGER IF EXISTS update_security_baselines_updated_at ON security_baselines;
DROP TRIGGER IF EXISTS update_config_templates_updated_at ON config_templates;
DROP TRIGGER IF EXISTS update_products_updated_at ON products;
DROP TRIGGER IF EXISTS update_instances_updated_at ON instances;
DROP TRIGGER IF EXISTS update_licenses_updated_at ON licenses;

-- Drop function
DROP FUNCTION IF EXISTS update_updated_at_column();

-- Drop tables (in order of dependencies)
DROP TABLE IF EXISTS heartbeat_history;
DROP TABLE IF EXISTS deployments;
DROP TABLE IF EXISTS releases;
DROP TABLE IF EXISTS instances;
DROP TABLE IF EXISTS licenses;
DROP TABLE IF EXISTS security_baselines;
DROP TABLE IF EXISTS config_templates;
DROP TABLE IF EXISTS products;

