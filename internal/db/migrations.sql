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
    updated_at      TIMESTAMP DEFAULT NOW()
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
