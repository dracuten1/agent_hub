-- ============================================================================
-- AgentHub Database Schema
-- ============================================================================

-- 001_users.sql
CREATE TABLE IF NOT EXISTS users (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    username    VARCHAR(100) UNIQUE NOT NULL,
    email       VARCHAR(200) UNIQUE NOT NULL,
    password    VARCHAR(255) NOT NULL,
    role        VARCHAR(20) DEFAULT 'user',
    created_at  TIMESTAMP DEFAULT NOW()
);

-- 002_agents.sql
CREATE TABLE IF NOT EXISTS agents (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    name            VARCHAR(100) UNIQUE NOT NULL,
    role            VARCHAR(50) NOT NULL,
    skills          TEXT[] DEFAULT '{}',
    api_key         VARCHAR(255) UNIQUE NOT NULL,
    status          VARCHAR(20) DEFAULT 'idle',
    last_heartbeat  TIMESTAMP,
    current_tasks   INT DEFAULT 0,
    max_tasks       INT DEFAULT 3,
    total_completed INT DEFAULT 0,
    total_failed    INT DEFAULT 0,
    model           VARCHAR(100),
    tool            VARCHAR(100),
    created_at      TIMESTAMP DEFAULT NOW()
);

-- 003_projects.sql
CREATE TABLE IF NOT EXISTS projects (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    name        VARCHAR(200) NOT NULL,
    description TEXT DEFAULT '',
    user_id     TEXT REFERENCES users(id),
    status      VARCHAR(20) DEFAULT 'active',
    created_at  TIMESTAMP DEFAULT NOW(),
    updated_at  TIMESTAMP DEFAULT NOW()
);

-- 004_features.sql
CREATE TABLE IF NOT EXISTS features (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    project_id  TEXT REFERENCES projects(id) ON DELETE CASCADE,
    name        VARCHAR(200) NOT NULL,
    description TEXT DEFAULT '',
    user_id     TEXT REFERENCES users(id),
    status      VARCHAR(20) DEFAULT 'active',
    created_at  TIMESTAMP DEFAULT NOW(),
    updated_at  TIMESTAMP DEFAULT NOW()
);

-- 005_tasks.sql
CREATE TABLE IF NOT EXISTS tasks (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    project_id      TEXT REFERENCES projects(id) ON DELETE SET NULL,
    feature_id      TEXT REFERENCES features(id) ON DELETE SET NULL,
    title           VARCHAR(500) NOT NULL,
    description     TEXT DEFAULT '',
    priority        VARCHAR(20) DEFAULT 'medium',
    status          VARCHAR(30) DEFAULT 'available',
    assignee        VARCHAR(100),
    required_skills TEXT[] DEFAULT '{}',
    retry_count     INT DEFAULT 0,
    max_retries     INT DEFAULT 2,
    progress        INT DEFAULT 0,
    review_verdict  VARCHAR(20),
    review_severity VARCHAR(20),
    review_issues   TEXT[] NOT NULL DEFAULT '{}',
    test_verdict    VARCHAR(20),
    test_issues     TEXT[] NOT NULL DEFAULT '{}',
    escalated       BOOLEAN DEFAULT false,
    claimed_at      TIMESTAMP,
    completed_at    TIMESTAMP,
    deadline        TIMESTAMP,
    created_by      TEXT REFERENCES users(id),
    user_id         TEXT REFERENCES users(id),
    created_at      TIMESTAMP DEFAULT NOW(),
    updated_at      TIMESTAMP DEFAULT NOW(),
    -- 010: stale task tracking
    release_count   INT DEFAULT 0,
    -- dependency tracking
    pending_deps    INT DEFAULT 0
);

-- 006_task_events.sql
CREATE TABLE IF NOT EXISTS task_events (
    id          BIGSERIAL PRIMARY KEY,
    task_id     TEXT REFERENCES tasks(id) ON DELETE CASCADE,
    agent       VARCHAR(100),
    event       VARCHAR(50) NOT NULL,
    from_status VARCHAR(30),
    to_status   VARCHAR(30),
    note        TEXT,
    metadata    JSONB DEFAULT '{}',
    created_at  TIMESTAMP DEFAULT NOW()
);

-- 007_comments.sql
CREATE TABLE IF NOT EXISTS comments (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    task_id     TEXT REFERENCES tasks(id) ON DELETE CASCADE,
    user_id     TEXT REFERENCES users(id),
    agent       VARCHAR(100),
    content     TEXT NOT NULL,
    created_at  TIMESTAMP DEFAULT NOW()
);

-- 008_task_dependencies.sql
CREATE TABLE IF NOT EXISTS task_dependencies (
    id              BIGSERIAL PRIMARY KEY,
    task_id         TEXT REFERENCES tasks(id) ON DELETE CASCADE,
    depends_on_id   TEXT REFERENCES tasks(id) ON DELETE CASCADE,
    created_at      TIMESTAMP DEFAULT NOW(),
    UNIQUE(task_id, depends_on_id)
);

-- ============================================================================
-- Workflow Tables
-- ============================================================================

-- workflows: top-level workflow instances
CREATE TABLE IF NOT EXISTS workflows (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    name        VARCHAR(200) NOT NULL,
    project_id  TEXT REFERENCES projects(id) ON DELETE SET NULL,
    total_phases INT NOT NULL DEFAULT 0,
    status      VARCHAR(30) DEFAULT 'active',
    created_at  TIMESTAMP DEFAULT NOW(),
    updated_at  TIMESTAMP DEFAULT NOW()
);

-- workflow_phases: individual phases within a workflow
CREATE TABLE IF NOT EXISTS workflow_phases (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    workflow_id TEXT REFERENCES workflows(id) ON DELETE CASCADE,
    phase_name  VARCHAR(200) NOT NULL,
    phase_index INT NOT NULL DEFAULT 0,
    phase_type  VARCHAR(30) NOT NULL DEFAULT 'general',
    status      VARCHAR(30) DEFAULT 'pending',
    created_at  TIMESTAMP DEFAULT NOW(),
    updated_at  TIMESTAMP DEFAULT NOW()
);

-- workflow_task_map: maps tasks to workflow phases
CREATE TABLE IF NOT EXISTS workflow_task_map (
    id          BIGSERIAL PRIMARY KEY,
    task_id     TEXT REFERENCES tasks(id) ON DELETE CASCADE,
    workflow_id TEXT REFERENCES workflows(id) ON DELETE CASCADE,
    phase_id    TEXT REFERENCES workflow_phases(id) ON DELETE SET NULL,
    created_at  TIMESTAMP DEFAULT NOW(),
    UNIQUE(task_id)
);

-- ============================================================================
-- Indexes
-- ============================================================================

-- 008_indexes.sql
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_assignee ON tasks(assignee);
CREATE INDEX IF NOT EXISTS idx_tasks_priority ON tasks(priority);
CREATE INDEX IF NOT EXISTS idx_tasks_project ON tasks(project_id);
CREATE INDEX IF NOT EXISTS idx_tasks_feature ON tasks(feature_id);
CREATE INDEX IF NOT EXISTS idx_agents_status ON agents(status);
CREATE INDEX IF NOT EXISTS idx_agents_name ON agents(name);
CREATE INDEX IF NOT EXISTS idx_task_events_task ON task_events(task_id);
CREATE INDEX IF NOT EXISTS idx_comments_task ON comments(task_id);
CREATE INDEX IF NOT EXISTS idx_features_project ON features(project_id);

-- 009_add_task_type.sql
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS task_type VARCHAR(20) DEFAULT 'general';
CREATE INDEX IF NOT EXISTS idx_tasks_task_type ON tasks(task_type);

-- 010_stale_task_indexes.sql
CREATE INDEX IF NOT EXISTS idx_tasks_stale_check
  ON tasks (status, claimed_at, updated_at)
  WHERE status IN ('claimed', 'in_progress');

-- ============================================================================
-- Triggers
-- ============================================================================

-- fn_dep_added: increment pending_deps when dependency added
CREATE OR REPLACE FUNCTION fn_dep_added()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE tasks SET pending_deps = pending_deps + 1 WHERE id = NEW.task_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_dep_added ON task_dependencies;
CREATE TRIGGER trg_dep_added
    AFTER INSERT ON task_dependencies
    FOR EACH ROW EXECUTE FUNCTION fn_dep_added();
