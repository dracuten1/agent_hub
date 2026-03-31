-- 010_task_claim_timeout.sql
-- Orphan recovery: partial index + claim tracking + release endpoint

-- Partial index: only claimed tasks need claim-timeout tracking
CREATE INDEX IF NOT EXISTS idx_tasks_claimed_timeout
ON tasks(claimed_at, updated_at)
WHERE status = 'claimed';

-- Release count: how many times a task has been released back to the queue
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS release_count INTEGER DEFAULT 0;

-- Orphan threshold (minutes): tasks claimed but not updated within this window
-- are considered orphaned and eligible for re-queue by the monitor.
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS orphan_threshold_minutes INTEGER DEFAULT 60;
