import { useState } from 'react';
import { CheckCircle, Clock, XCircle, Circle, ChevronDown } from 'lucide-react';
import type { PhaseWithTasks, Task } from '../types';
import { StatusBadge, ProgressBar } from './shared';
import { TaskArtifact } from './TaskArtifact';
import styles from './PhaseNode.module.css';

interface Props {
  phase: PhaseWithTasks;
  isCurrent: boolean;
}

const icons: Record<string, React.FC<{ size?: number; className?: string }>> = {
  completed: CheckCircle,
  running:   Clock,
  failed:    XCircle,
};

function phaseDescription(phase: PhaseWithTasks): string {
  if (typeof phase.config === 'object' && phase.config !== null && 'description' in phase.config) {
    return String(phase.config['description']);
  }
  return `${phase.task_type} phase — ${phase.phase_type}`;
}

// ── Task row ────────────────────────────────────────────────────────────────
function TaskRow({ task }: { task: Task }) {
  const [open, setOpen] = useState(false);

  return (
    <div className={styles.taskItem}>
      <div className={styles.taskRow} onClick={() => setOpen(o => !o)}>
        <ChevronDown
          size={12}
          className={`${styles.chevron} ${open ? styles.chevronOpen : ''}`}
        />
        <span className={styles.taskTitle}>{task.title}</span>
        <div className={styles.taskMeta}>
          {task.assignee && <span className={styles.assignee}>{task.assignee}</span>}
          <StatusBadge status={task.status} size="sm" />
        </div>
      </div>
      <div className={`${styles.taskBody} ${open ? styles.taskBodyOpen : ''}`}>
        <TaskArtifact task={task} />
      </div>
    </div>
  );
}

// ── Main component ──────────────────────────────────────────────────────────
export function PhaseNode({ phase, isCurrent }: Props) {
  const [expanded, setExpanded] = useState(false);

  const Icon    = icons[phase.status] ?? Circle;
  const done    = phase.completed_tasks ?? 0;
  const total   = phase.total_tasks ?? phase.tasks?.length ?? 0;
  const pct     = total > 0 ? Math.round((done / total) * 100) : 0;
  const desc    = phaseDescription(phase);
  const hasTasks = phase.tasks && phase.tasks.length > 0;

  return (
    <div className={`${styles.node} ${styles[phase.status] ?? ''} ${isCurrent ? styles.current : ''}`}>
      {/* Header — clickable to expand/collapse */}
      <div className={styles.header} onClick={() => setExpanded(e => !e)}>
        <Icon size={15} className={styles.icon} />
        <span className={styles.phaseName}>{phase.name}</span>
        <StatusBadge status={phase.status} size="sm" />
        <ChevronDown
          size={14}
          className={`${styles.expandIcon} ${expanded ? styles.expandIconOpen : ''}`}
        />
      </div>

      <div className={styles.meta}>
        <span className={styles.taskType}>{phase.task_type}</span>
        <span className={styles.taskCount}>{done}/{total} tasks</span>
      </div>

      <ProgressBar value={pct} size="sm" />

      {/* Expanded content */}
      <div className={`${styles.expanded} ${expanded ? styles.expandedOpen : ''}`}>
        {/* Phase description */}
        <p className={styles.phaseDesc}>{desc}</p>

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
