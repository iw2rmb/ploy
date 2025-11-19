-- API tokens (long-lived, for CLI usage)
CREATE TABLE api_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash VARCHAR(64) NOT NULL UNIQUE,  -- SHA-256 of full token
    token_id VARCHAR(32) NOT NULL,           -- JWT "jti" claim for lookup
    cluster_id VARCHAR(100) NOT NULL,
    role VARCHAR(50) NOT NULL,
    description TEXT,                         -- User-provided description
    issued_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_by VARCHAR(255),                  -- Which user created it
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_api_tokens_token_id ON api_tokens(token_id);
CREATE INDEX idx_api_tokens_cluster_id ON api_tokens(cluster_id);

-- Bootstrap tokens (short-lived, for node provisioning)
CREATE TABLE bootstrap_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash VARCHAR(64) NOT NULL UNIQUE,
    token_id VARCHAR(32) NOT NULL,
    node_id UUID NOT NULL,
    cluster_id VARCHAR(100) NOT NULL,
    issued_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,                      -- NULL until used
    cert_issued_at TIMESTAMPTZ,               -- NULL if cert issuance failed
    revoked_at TIMESTAMPTZ,
    issued_by VARCHAR(255),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_bootstrap_tokens_token_id ON bootstrap_tokens(token_id);
CREATE INDEX idx_bootstrap_tokens_node_id ON bootstrap_tokens(node_id);
