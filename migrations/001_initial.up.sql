-- MySoc Updates Platform - Initial Schema
-- Run with: psql -d mysoc_updates -f migrations/001_initial.up.sql

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Licenses table
CREATE TABLE IF NOT EXISTS licenses (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    license_key VARCHAR(50) UNIQUE NOT NULL,
    customer_id VARCHAR(100) NOT NULL,
    customer_name VARCHAR(255) NOT NULL,
    license_type VARCHAR(50) NOT NULL,  -- mysoc-cloud, siemcore, siemcore-lite
    products TEXT[] NOT NULL DEFAULT '{}',
    features TEXT[] DEFAULT '{}',
    limits JSONB DEFAULT '{}',
    issued_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    bound_to VARCHAR(255),              -- Optional hardware binding
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Instances table (registered servers)
CREATE TABLE IF NOT EXISTS instances (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    instance_id VARCHAR(100) UNIQUE NOT NULL,
    instance_type VARCHAR(50) NOT NULL, -- mysoc, siemcore
    hostname VARCHAR(255),
    license_id UUID REFERENCES licenses(id) ON DELETE SET NULL,
    api_key_hash VARCHAR(64) NOT NULL,
    last_heartbeat TIMESTAMP WITH TIME ZONE,
    last_heartbeat_data JSONB,
    status VARCHAR(20) DEFAULT 'unknown', -- online, offline, degraded
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Releases table
CREATE TABLE IF NOT EXISTS releases (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    product_name VARCHAR(100) NOT NULL,
    version VARCHAR(50) NOT NULL,
    channel VARCHAR(20) DEFAULT 'stable', -- stable, beta, nightly
    manifest JSONB NOT NULL DEFAULT '{}',
    artifact_path VARCHAR(500),
    artifact_size BIGINT DEFAULT 0,
    checksum VARCHAR(64),
    signature VARCHAR(500),
    release_notes TEXT,
    min_updater_version VARCHAR(50),
    released_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(product_name, version)
);

-- Deployments tracking
CREATE TABLE IF NOT EXISTS deployments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    instance_id UUID REFERENCES instances(id) ON DELETE CASCADE,
    release_id UUID REFERENCES releases(id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL,  -- pending, downloading, installing, success, failed, rolled_back
    started_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    completed_at TIMESTAMP WITH TIME ZONE,
    error_message TEXT,
    previous_version VARCHAR(50)
);

-- Heartbeat history (for analytics)
CREATE TABLE IF NOT EXISTS heartbeat_history (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    instance_id UUID REFERENCES instances(id) ON DELETE CASCADE,
    heartbeat_data JSONB NOT NULL,
    received_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Products table (registered products)
CREATE TABLE IF NOT EXISTS products (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(100) UNIQUE NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    description TEXT,
    type VARCHAR(50) NOT NULL, -- binary, data, config
    default_channel VARCHAR(20) DEFAULT 'stable',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Config templates
CREATE TABLE IF NOT EXISTS config_templates (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(100) UNIQUE NOT NULL,
    description TEXT,
    template JSONB NOT NULL,
    variables JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Security baselines
CREATE TABLE IF NOT EXISTS security_baselines (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(100) UNIQUE NOT NULL,
    description TEXT,
    checks JSONB NOT NULL DEFAULT '[]',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_instances_status ON instances(status);
CREATE INDEX IF NOT EXISTS idx_instances_license_id ON instances(license_id);
CREATE INDEX IF NOT EXISTS idx_instances_last_heartbeat ON instances(last_heartbeat);
CREATE INDEX IF NOT EXISTS idx_releases_product_channel ON releases(product_name, channel);
CREATE INDEX IF NOT EXISTS idx_releases_released_at ON releases(released_at DESC);
CREATE INDEX IF NOT EXISTS idx_deployments_instance_id ON deployments(instance_id);
CREATE INDEX IF NOT EXISTS idx_deployments_status ON deployments(status);
CREATE INDEX IF NOT EXISTS idx_heartbeat_history_instance ON heartbeat_history(instance_id, received_at DESC);
CREATE INDEX IF NOT EXISTS idx_licenses_customer_id ON licenses(customer_id);
CREATE INDEX IF NOT EXISTS idx_licenses_expires_at ON licenses(expires_at);

-- Function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Triggers for updated_at
CREATE TRIGGER update_licenses_updated_at
    BEFORE UPDATE ON licenses
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_instances_updated_at
    BEFORE UPDATE ON instances
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_products_updated_at
    BEFORE UPDATE ON products
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_config_templates_updated_at
    BEFORE UPDATE ON config_templates
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_security_baselines_updated_at
    BEFORE UPDATE ON security_baselines
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Insert default products
INSERT INTO products (name, display_name, description, type) VALUES
    ('siemcore-api', 'SIEMCore API', 'SIEMCore API Server', 'binary'),
    ('siemcore-collector', 'SIEMCore Collector', 'SIEMCore Log Collector', 'binary'),
    ('siemcore-frontend', 'SIEMCore Frontend', 'SIEMCore Web UI', 'binary'),
    ('detection-rules', 'Detection Rules', 'SIEM Detection Rules', 'data'),
    ('mysoc-api', 'MySoc API', 'MySoc Platform API', 'binary'),
    ('mysoc-frontend', 'MySoc Frontend', 'MySoc Web UI', 'binary')
ON CONFLICT (name) DO NOTHING;

-- Insert default config templates
INSERT INTO config_templates (name, description, template, variables) VALUES
    ('siemcore-standard', 'Standard SIEMCore Configuration', '{
        "api": {
            "port": 8080,
            "host": "0.0.0.0"
        },
        "collector": {
            "port": 5514,
            "protocols": ["syslog", "json"]
        },
        "database": {
            "type": "postgresql"
        }
    }', '{"hostname": "", "domain": ""}'),
    ('mysoc-cloud', 'MySoc Cloud Configuration', '{
        "api": {
            "port": 8080,
            "host": "0.0.0.0"
        },
        "multi_tenant": true
    }', '{"hostname": "", "domain": ""}')
ON CONFLICT (name) DO NOTHING;

-- Insert default security baseline
INSERT INTO security_baselines (name, description, checks) VALUES
    ('cis-level1', 'CIS Level 1 Security Baseline', '[
        {"id": "fw-enabled", "name": "Firewall Enabled", "type": "firewall"},
        {"id": "ssh-root-disabled", "name": "SSH Root Login Disabled", "type": "ssh"},
        {"id": "ssh-password-disabled", "name": "SSH Password Auth Disabled", "type": "ssh"},
        {"id": "updates-current", "name": "Security Updates Applied", "type": "os"},
        {"id": "tls-valid", "name": "TLS Certificates Valid", "type": "tls"}
    ]')
ON CONFLICT (name) DO NOTHING;

