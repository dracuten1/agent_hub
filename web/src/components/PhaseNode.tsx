import { useState } from 'react';
import { CheckCircle, Clock, XCircle, Circle, ChevronDown, Lock } from 'lucide-react';
import type { PhaseWithTasks, Task } from '../types';
import { StatusBadge } from './shared';
import { TaskArtifact } from './TaskArtifact';
import { apiApprovePhase } from '../api/client';
import styles from './PhaseNode.module.css';

// ── Phase subtype helpers ─────────────────────────────────────────────────────

interface PhaseConfig {
  require_owner?: boolean;
  approver?: string;
  threshold?: number;
  [key: string]: unknown;
}

function parseConfig(config: unknown): PhaseConfig {
  if (typeof config === 'object' && config !== null && !Array.isArray(config)) {
    return config as PhaseConfig;
  }
  return {};
}

function getPhaseSubtype(phase: PhaseWithTasks): string {
  const cfg = parseConfig(phase.config);
  if (phase.phase_type === 'gate') {
    if (cfg.require_owner || cfg.approver === 'owner') return 'human_gate';
    return 'agent_gate';
  }
  if (phase.phase_type === 'decision') return 'decision';
  if (phase.phase_type === 'per_dev') return 'per_dev';
  return phase.phase_type ?? 'normal';
}

function SubtypeBadge({ subtype }: { subtype: string }) {
  const map: Record<string, { label: string; cls: string }> = {
    agent_gate:  { label: 'Agent Gate', cls: styles.badgeAgentGate },
    human_gate:  { label: 'Human Gate', cls: styles.badgeHumanGate },
    decision:    { label: 'Decision',    cls: styles.badgeDecision },
    per_dev:     { label: 'per_dev',     cls: styles.badgePerDev },
    normal:      { label: 'Normal',      cls: styles.badgeNormal },
    single:      { label: 'Single',       cls: styles.badgeNormal },
    multi:       { label: 'Multi',        cls: styles.badgeNormal },
  };
  const { label, cls } = map[subtype] ?? { label: subtype, cls: styles.badgeNormal };
  const isHuman = subtype === 'human_gate';
  return (
    <span className={`${styles.subtypeBadge} ${cls}`}>
      {isHuman && <Lock size={9} />}
      {label}
    </span>
  );
}

// ── PM decision display ───────────────────────────────────────────────────────

function PMDecision({ task }: { task: Task }) {
  const verdict = task.review_verdict ?? task.status === 'done' ? 'pass' : 'fail';
  const issues = task.review_issues ?? [];
  const isPass = verdict === 'pass';

  return (
    <div className={styles.decisionBox}>
      <div className={styles.decisionRow}>
        <span className={`${styles.decisionVerdict} ${isPass ? styles.verdictPass : styles.verdictFail}`}>
          {isPass ? '✓ PASS' : '✗ FAIL'}
        </span>
        {task.assignee && (
          <span className={styles.decisionAssignee}>
            by {task.assignee}
          </span>
        )}
      </div>
      {issues.length > 0 && (
        <ul className={styles.decisionIssues}>
          {issues.map((issue, i) => (
            <li key={i}>{issue}</li>
          ))}
        </ul>
      )}
      {issues.length === 0 && task.result && (
        <p className={styles.decisionResult}>{task.result}</p>
      )}
    </div>
  );
}

// ── Task row ─────────────────────────────────────────────────────────────────

function TaskRow({ task }: { task: Task }) {
  const [open, setOpen] = useState(false);
  const isGateDecision = task.type === 'gate_decision' || task.title?.toLowerCase().includes('gate decision');

  return (
    <div className={styles.taskItem}>
      <div className={styles.taskRow} onClick={() => setOpen(o => !o)}>
        <ChevronDown
          size={12}
          className={`${styles.chevron} ${open ? styles.chevronOpen : ''}`}
        />
        <span className={styles.taskTitle}>{task.title}</span>
        <div className={styles.taskMeta}>
          {task.auto_created && (
            <span className={styles.autoBadge}>auto</span>
          )}
          {task.assignee && <span className={styles.assignee}>{task.assignee}</span>}
          {isGateDecision ? (
            <StatusBadge status={task.review_verdict ?? 'pending'} size="sm" />
          ) : (
            <StatusBadge status={task.status} size="sm" />
          )}
        </div>
      </div>
      <div className={`${styles.taskBody} ${open ? styles.taskBodyOpen : ''}`}>
        {isGateDecision && (task.review_verdict || task.review_issues?.length) && (
          <PMDecision task={task} />
        )}
        <TaskArtifact task={task} />
      </div>
    </div>
  );
}

// ── Phase subtype description ─────────────────────────────────────────────────

function phaseDescription(phase: PhaseWithTasks): string {
  const cfg = parseConfig(phase.config);
  if (typeof phase.config === 'object' && phase.config !== null) {
    if ('description' in phase.config && phase.config['description']) {
      return String(phase.config['description']);
    }
  }
  if (phase.phase_type === 'gate') {
    if (cfg.require_owner || cfg.approver === 'owner') {
      return 'Human gate — requires explicit approval before proceeding.';
    }
    return 'Agent gate — PM agent makes the go/no-go decision.';
  }
  if (phase.phase_type === 'decision') {
    const threshold = cfg.threshold;
    if (threshold) return `Decision gate (${threshold}% threshold required to pass).`;
    return 'Decision gate — evaluates previous phase results to decide next step.';
  }
  if (phase.phase_type === 'per_dev') {
    return 'Per-developer phase — one task per developer, auto-created from previous phase.';
  }
  return `${phase.task_type} phase — ${phase.phase_type}`;
}

// ── Main component ───────────────────────────────────────────────────────────

interface Props {
  phase: PhaseWithTasks;
  isCurrent: boolean;
  workflowId?: string;
}

export function PhaseNode({ phase, isCurrent, workflowId }: Props) {
  const [expanded, setExpanded] = useState(isCurrent);
  const [approveLoading, setApproveLoading] = useState(false);
  const [approveError, setApproveError] = useState('');

  const Icon        = { completed: CheckCircle, running: Clock, failed: XCircle }[phase.status] ?? Circle;
  const done        = phase.completed_tasks ?? 0;
  const total       = phase.total_tasks ?? phase.tasks?.length ?? 0;
  const pct         = total > 0 ? Math.round((done / total) * 100) : 0;
  const desc        = phaseDescription(phase);
  const hasTasks    = phase.tasks && phase.tasks.length > 0;
  const subtype     = getPhaseSubtype(phase);
  const isWaiting   = phase.status === 'waiting_approval';
  const gateTask    = phase.tasks?.find(t =>
    t.type === 'gate_decision' || t.title?.toLowerCase().includes('gate decision')
  );
  const isHumanGate = subtype === 'human_gate';

  async function handleApprove() {
    if (!workflowId || !phase.id) return;
    setApproveError('');
    setApproveLoading(true);
    try {
      await apiApprovePhase(workflowId, phase.id);
      window.location.reload();
    } catch (e) {
      setApproveError(e instanceof Error ? e.message : 'Approval failed');
    } finally {
      setApproveLoading(false);
    }
  }

  return (
    <div className={`${styles.node} ${styles[phase.status] ?? ''} ${isCurrent ? styles.current : ''}`}>
      {/* Header — clickable to expand/collapse */}
      <div className={styles.header} onClick={() => setExpanded(e => !e)}>
        <Icon size={15} className={styles.icon} />
        <span className={styles.phaseName}>{phase.name}</span>
        <SubtypeBadge subtype={subtype} />
        {isWaiting && <StatusBadge status="waiting_approval" size="sm" />}
        {!isWaiting && <StatusBadge status={phase.status} size="sm" />}
        <ChevronDown
          size={14}
          className={`${styles.expandIcon} ${expanded ? styles.expandIconOpen : ''}`}
        />
      </div>

      <div className={styles.meta}>
        <span className={styles.taskType}>{phase.task_type}</span>
        <span className={styles.taskCount}>
          {subtype === 'per_dev' && total > 0
            ? `${total} tasks`
            : `${done}/${total} tasks`}
        </span>
      </div>

      <div className={styles.progressTrack}>
        <div
          className={`${styles.progressFill} ${styles[`fill_${phase.status}`] ?? styles.fillDefault}`}
          style={{ width: `${pct}%` }}
        />
      </div>

      {/* Expanded content */}
      <div className={`${styles.expanded} ${expanded ? styles.expandedOpen : ''}`}>
        <p className={styles.phaseDesc}>{desc}</p>

        {/* PM decision display */}
        {gateTask && (gateTask.review_verdict || gateTask.review_issues?.length) && (
          <PMDecision task={gateTask} />
        )}

        {/* Human gate approval button */}
        {isHumanGate && isWaiting && (
          <div className={styles.approveSection}>
            {approveError && (
              <p className={styles.approveError}>{approveError}</p>
            )}
            <button
              className={styles.approveBtn}
              onClick={handleApprove}
              disabled={approveLoading}
            >
              {approveLoading ? 'Approving…' : '✓ Approve Gate'}
            </button>
          </div>
        )}

        {/* Task list */}
        {hasTasks ? (
          <div className={styles.taskList}>
            {phase.tasks!.map(task => (
              <TaskRow key={task.id} task={task} />
            ))}
          </div>
        ) : (
          <p className={styles.noTasks}>No tasks in this phase</p>
        )}
      </div>
    </div>
  );
}
