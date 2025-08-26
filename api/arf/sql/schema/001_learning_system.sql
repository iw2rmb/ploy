-- ARF Phase 3: Learning System Database Schema
-- This schema supports the continuous learning and pattern extraction system

-- Transformation outcomes for learning
CREATE TABLE IF NOT EXISTS transformation_outcomes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transformation_id TEXT NOT NULL,
    recipe_id TEXT NOT NULL,
    success BOOLEAN NOT NULL,
    duration_seconds INTEGER NOT NULL,
    language TEXT NOT NULL,
    framework TEXT,
    pattern_signature TEXT NOT NULL,
    codebase_size INTEGER,
    complexity_score DECIMAL(3,2),
    strategy TEXT NOT NULL,
    error_type TEXT,
    error_message TEXT,
    performance_impact DECIMAL(5,4),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    metadata JSONB DEFAULT '{}'::jsonb
);

-- Success patterns identified from outcomes
CREATE TABLE IF NOT EXISTS success_patterns (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    signature TEXT UNIQUE NOT NULL,
    language TEXT NOT NULL,
    success_rate DECIMAL(5,4) NOT NULL,
    occurrence_count INTEGER NOT NULL DEFAULT 1,
    avg_duration DECIMAL(8,2) NOT NULL,
    confidence_level DECIMAL(3,2) NOT NULL,
    factors JSONB NOT NULL DEFAULT '{}'::jsonb,
    conditions JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Failure patterns for anti-pattern detection
CREATE TABLE IF NOT EXISTS failure_patterns (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    signature TEXT UNIQUE NOT NULL,
    frequency INTEGER NOT NULL DEFAULT 1,
    failure_rate DECIMAL(5,4) NOT NULL,
    common_errors TEXT[] NOT NULL DEFAULT '{}',
    context_factors TEXT[] NOT NULL DEFAULT '{}',
    mitigations TEXT[] NOT NULL DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Recipe templates generated from successful patterns
CREATE TABLE IF NOT EXISTS pattern_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    pattern_signature TEXT NOT NULL,
    language TEXT NOT NULL,
    success_rate DECIMAL(5,4) NOT NULL,
    usage_count INTEGER NOT NULL DEFAULT 0,
    template_recipe JSONB NOT NULL,
    confidence_threshold DECIMAL(3,2) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    last_used TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    FOREIGN KEY (pattern_signature) REFERENCES success_patterns(signature)
);

-- Strategy weights optimization
CREATE TABLE IF NOT EXISTS strategy_weights (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    strategy_name TEXT NOT NULL,
    language TEXT NOT NULL,
    pattern_type TEXT NOT NULL,
    weight DECIMAL(5,4) NOT NULL,
    performance_score DECIMAL(5,4) NOT NULL,
    sample_size INTEGER NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(strategy_name, language, pattern_type)
);

-- Learning analytics and insights
CREATE TABLE IF NOT EXISTS learning_insights (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    insight_type TEXT NOT NULL,
    language TEXT,
    framework TEXT,
    description TEXT NOT NULL,
    confidence DECIMAL(3,2) NOT NULL,
    supporting_data JSONB NOT NULL,
    actionable BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_transformation_outcomes_pattern ON transformation_outcomes(pattern_signature);
CREATE INDEX IF NOT EXISTS idx_transformation_outcomes_language ON transformation_outcomes(language);
CREATE INDEX IF NOT EXISTS idx_transformation_outcomes_success ON transformation_outcomes(success);
CREATE INDEX IF NOT EXISTS idx_transformation_outcomes_created_at ON transformation_outcomes(created_at);

CREATE INDEX IF NOT EXISTS idx_success_patterns_language ON success_patterns(language);
CREATE INDEX IF NOT EXISTS idx_success_patterns_success_rate ON success_patterns(success_rate);
CREATE INDEX IF NOT EXISTS idx_success_patterns_updated_at ON success_patterns(updated_at);

CREATE INDEX IF NOT EXISTS idx_failure_patterns_signature ON failure_patterns(signature);
CREATE INDEX IF NOT EXISTS idx_failure_patterns_failure_rate ON failure_patterns(failure_rate);

CREATE INDEX IF NOT EXISTS idx_pattern_templates_pattern ON pattern_templates(pattern_signature);
CREATE INDEX IF NOT EXISTS idx_pattern_templates_language ON pattern_templates(language);
CREATE INDEX IF NOT EXISTS idx_pattern_templates_usage ON pattern_templates(usage_count);

CREATE INDEX IF NOT EXISTS idx_strategy_weights_strategy ON strategy_weights(strategy_name);
CREATE INDEX IF NOT EXISTS idx_strategy_weights_language ON strategy_weights(language);