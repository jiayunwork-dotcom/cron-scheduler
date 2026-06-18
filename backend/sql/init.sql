-- PostgreSQL 16 初始化脚本 - Cron 调度系统

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ==================== updated_at 自动更新触发器 ====================

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ==================== tasks 表 ====================

CREATE TABLE IF NOT EXISTS tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL UNIQUE,
    cron_expr VARCHAR(255) NOT NULL,
    command TEXT NOT NULL,
    timeout_sec INT DEFAULT 60,
    max_retries INT DEFAULT 0,
    retry_strategy VARCHAR(20) DEFAULT 'fixed',
    retry_interval_sec INT DEFAULT 60,
    priority INT DEFAULT 5 CHECK (priority BETWEEN 1 AND 10),
    dependencies TEXT[] DEFAULT '{}'::TEXT[],
    trigger_condition VARCHAR(20) DEFAULT 'all_success',
    enabled BOOLEAN DEFAULT TRUE,
    tags TEXT[] DEFAULT '{}'::TEXT[],
    compensation VARCHAR(20) DEFAULT 'skip',
    alert_enabled BOOLEAN DEFAULT TRUE,
    last_run_at TIMESTAMPTZ NULL,
    next_run_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_tasks_name ON tasks(name);
CREATE INDEX IF NOT EXISTS idx_tasks_enabled ON tasks(enabled);

CREATE TRIGGER update_tasks_updated_at
BEFORE UPDATE ON tasks
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

-- ==================== execution_history 表 ====================

CREATE TABLE IF NOT EXISTS execution_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id UUID REFERENCES tasks(id) ON DELETE CASCADE,
    task_name VARCHAR(255) NOT NULL,
    trigger_type VARCHAR(20) DEFAULT 'cron',
    trigger_time TIMESTAMPTZ NOT NULL,
    start_time TIMESTAMPTZ NULL,
    end_time TIMESTAMPTZ NULL,
    duration_ms BIGINT NULL,
    status VARCHAR(20) DEFAULT 'pending',
    exit_code INT NULL,
    stdout TEXT DEFAULT '',
    stderr TEXT DEFAULT '',
    retry_count INT DEFAULT 0,
    error_message TEXT DEFAULT '',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_exec_task_id ON execution_history(task_id);
CREATE INDEX IF NOT EXISTS idx_exec_task_name ON execution_history(task_name);
CREATE INDEX IF NOT EXISTS idx_exec_status ON execution_history(status);
CREATE INDEX IF NOT EXISTS idx_exec_created ON execution_history(created_at DESC);

-- ==================== alerts 表 ====================

CREATE TABLE IF NOT EXISTS alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id UUID REFERENCES tasks(id) ON DELETE CASCADE,
    task_name VARCHAR(255) NOT NULL,
    execution_id UUID REFERENCES execution_history(id) ON DELETE CASCADE,
    alert_type VARCHAR(20) NOT NULL,
    message TEXT DEFAULT '',
    webhook_url VARCHAR(1024) DEFAULT '',
    sent BOOLEAN DEFAULT FALSE,
    sent_at TIMESTAMPTZ NULL,
    error_message TEXT DEFAULT '',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_alerts_task_id ON alerts(task_id);
CREATE INDEX IF NOT EXISTS idx_alerts_sent ON alerts(sent);

-- ==================== system_settings 表 ====================

CREATE TABLE IF NOT EXISTS system_settings (
    key VARCHAR(100) PRIMARY KEY,
    value TEXT DEFAULT '',
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TRIGGER update_system_settings_updated_at
BEFORE UPDATE ON system_settings
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

-- ==================== missed_executions 表 ====================

CREATE TABLE IF NOT EXISTS missed_executions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id UUID REFERENCES tasks(id) ON DELETE CASCADE,
    task_name VARCHAR(255) NOT NULL,
    scheduled_time TIMESTAMPTZ NOT NULL,
    detected_at TIMESTAMPTZ DEFAULT NOW(),
    compensation VARCHAR(20) DEFAULT 'skip',
    compensated BOOLEAN DEFAULT FALSE,
    compensated_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_missed_task_id ON missed_executions(task_id);
CREATE INDEX IF NOT EXISTS idx_missed_compensated ON missed_executions(compensated);

-- ==================== 默认系统设置 ====================

INSERT INTO system_settings (key, value) VALUES
    ('max_concurrent_jobs', '5'),
    ('default_timeout_sec', '60'),
    ('alert_webhook_url', ''),
    ('default_compensation', 'skip'),
    ('consecutive_failures_for_alert', '1'),
    ('alert_silent_minutes', '5')
ON CONFLICT (key) DO NOTHING;
