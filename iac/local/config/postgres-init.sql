-- PostgreSQL initialization script for Ploy local testing
-- This script sets up the test database schema and test data

-- Create extensions if they don't exist
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Create main application tables
CREATE TABLE IF NOT EXISTS applications (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL UNIQUE,
    language VARCHAR(50) NOT NULL,
    lane CHAR(1) NOT NULL CHECK (lane IN ('A', 'B', 'C', 'D', 'E', 'F', 'G')),
    status VARCHAR(50) DEFAULT 'created',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Create deployments table
CREATE TABLE IF NOT EXISTS deployments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    app_id UUID NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    version VARCHAR(100) NOT NULL,
    artifact_url TEXT,
    status VARCHAR(50) DEFAULT 'pending',
    deployed_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    health_check_url TEXT,
    environment JSONB DEFAULT '{}',
    
    CONSTRAINT unique_app_version UNIQUE(app_id, version)
);

-- Create build_logs table
CREATE TABLE IF NOT EXISTS build_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    app_id UUID NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    deployment_id UUID REFERENCES deployments(id) ON DELETE CASCADE,
    phase VARCHAR(50) NOT NULL,
    message TEXT NOT NULL,
    level VARCHAR(20) DEFAULT 'INFO',
    timestamp TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Create configuration table
CREATE TABLE IF NOT EXISTS app_configurations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    app_id UUID NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    key VARCHAR(255) NOT NULL,
    value TEXT,
    encrypted BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    CONSTRAINT unique_app_config UNIQUE(app_id, key)
);

-- Create metrics table for testing
CREATE TABLE IF NOT EXISTS app_metrics (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    app_id UUID NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    metric_name VARCHAR(255) NOT NULL,
    metric_value NUMERIC,
    metric_type VARCHAR(50) DEFAULT 'gauge',
    labels JSONB DEFAULT '{}',
    timestamp TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Create indexes for better performance
CREATE INDEX IF NOT EXISTS idx_applications_name ON applications(name);
CREATE INDEX IF NOT EXISTS idx_applications_lane ON applications(lane);
CREATE INDEX IF NOT EXISTS idx_applications_status ON applications(status);
CREATE INDEX IF NOT EXISTS idx_deployments_app_id ON deployments(app_id);
CREATE INDEX IF NOT EXISTS idx_deployments_status ON deployments(status);
CREATE INDEX IF NOT EXISTS idx_build_logs_app_id ON build_logs(app_id);
CREATE INDEX IF NOT EXISTS idx_build_logs_timestamp ON build_logs(timestamp);
CREATE INDEX IF NOT EXISTS idx_app_configurations_app_id ON app_configurations(app_id);
CREATE INDEX IF NOT EXISTS idx_app_metrics_app_id ON app_metrics(app_id);
CREATE INDEX IF NOT EXISTS idx_app_metrics_timestamp ON app_metrics(timestamp);

-- Insert test data for local development
INSERT INTO applications (id, name, language, lane, status) VALUES
    ('550e8400-e29b-41d4-a716-446655440001', 'test-go-app', 'go', 'B', 'deployed'),
    ('550e8400-e29b-41d4-a716-446655440002', 'test-node-app', 'javascript', 'E', 'building'),
    ('550e8400-e29b-41d4-a716-446655440003', 'test-java-app', 'java', 'C', 'deployed'),
    ('550e8400-e29b-41d4-a716-446655440004', 'test-python-app', 'python', 'E', 'failed'),
    ('550e8400-e29b-41d4-a716-446655440005', 'test-wasm-app', 'rust', 'G', 'created')
ON CONFLICT (name) DO NOTHING;

-- Insert test deployments
INSERT INTO deployments (id, app_id, version, artifact_url, status, health_check_url) VALUES
    ('660e8400-e29b-41d4-a716-446655440001', '550e8400-e29b-41d4-a716-446655440001', 'v1.0.0', 'http://localhost:8888/artifacts/test-go-app-v1.0.0.tar.gz', 'healthy', 'http://test-go-app.local.dev/health'),
    ('660e8400-e29b-41d4-a716-446655440002', '550e8400-e29b-41d4-a716-446655440002', 'v0.1.0', 'http://localhost:8888/artifacts/test-node-app-v0.1.0.tar.gz', 'building', null),
    ('660e8400-e29b-41d4-a716-446655440003', '550e8400-e29b-41d4-a716-446655440003', 'v2.1.0', 'http://localhost:8888/artifacts/test-java-app-v2.1.0.tar.gz', 'healthy', 'http://test-java-app.local.dev/actuator/health')
ON CONFLICT (app_id, version) DO NOTHING;

-- Insert test build logs
INSERT INTO build_logs (app_id, deployment_id, phase, message, level) VALUES
    ('550e8400-e29b-41d4-a716-446655440001', '660e8400-e29b-41d4-a716-446655440001', 'build', 'Starting Go build process', 'INFO'),
    ('550e8400-e29b-41d4-a716-446655440001', '660e8400-e29b-41d4-a716-446655440001', 'build', 'Compiling Go binary for Lane B (Unikraft)', 'INFO'),
    ('550e8400-e29b-41d4-a716-446655440001', '660e8400-e29b-41d4-a716-446655440001', 'deploy', 'Deploying to Nomad cluster', 'INFO'),
    ('550e8400-e29b-41d4-a716-446655440001', '660e8400-e29b-41d4-a716-446655440001', 'deploy', 'Health check passed', 'INFO'),
    ('550e8400-e29b-41d4-a716-446655440004', null, 'build', 'Python requirements.txt not found', 'ERROR'),
    ('550e8400-e29b-41d4-a716-446655440004', null, 'build', 'Build failed due to missing dependencies', 'ERROR');

-- Insert test configuration
INSERT INTO app_configurations (app_id, key, value, encrypted) VALUES
    ('550e8400-e29b-41d4-a716-446655440001', 'PORT', '8080', false),
    ('550e8400-e29b-41d4-a716-446655440001', 'LOG_LEVEL', 'info', false),
    ('550e8400-e29b-41d4-a716-446655440003', 'JAVA_OPTS', '-Xmx512m -Xms256m', false),
    ('550e8400-e29b-41d4-a716-446655440003', 'DATABASE_URL', 'postgresql://ploy:ploy-test@postgres:5432/ploy_test', true);

-- Insert test metrics
INSERT INTO app_metrics (app_id, metric_name, metric_value, metric_type, labels) VALUES
    ('550e8400-e29b-41d4-a716-446655440001', 'http_requests_total', 1547, 'counter', '{"method": "GET", "status": "200"}'),
    ('550e8400-e29b-41d4-a716-446655440001', 'memory_usage_bytes', 67108864, 'gauge', '{}'),
    ('550e8400-e29b-41d4-a716-446655440001', 'cpu_usage_percent', 15.5, 'gauge', '{}'),
    ('550e8400-e29b-41d4-a716-446655440003', 'jvm_memory_used_bytes', 134217728, 'gauge', '{"area": "heap"}'),
    ('550e8400-e29b-41d4-a716-446655440003', 'jvm_threads_current', 23, 'gauge', '{}');

-- Create updated_at trigger function
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Create triggers for updated_at columns
CREATE TRIGGER update_applications_updated_at BEFORE UPDATE ON applications
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_app_configurations_updated_at BEFORE UPDATE ON app_configurations
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Grant permissions to ploy user
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO ploy;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO ploy;
GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO ploy;