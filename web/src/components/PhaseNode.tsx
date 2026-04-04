import { CheckCircle, Clock, XCircle, Circle } from 'lucide-react';
import type { PhaseWithTasks, Task } from '../types';
import { StatusBadge, ProgressBar } from './shared';
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

function TaskRow({ task }: { task: Task }) {
  return (
    <div className={styles.taskRow}>
      <span className={styles.taskTitle}>{task.title}</span>
      <div className={styles.taskMeta}>
        {task.assignee && <span className={styles.assignee}>{task.assignee}</span>}
        <StatusBadge status={task.status} size="sm" />
      </div>
    </div>
  );
}

export function PhaseNode({ phase, isCurrent }: Props) {
  const Icon  = icons[phase.status] ?? Circle;
  const done  = phase.completed_tasks ?? 0;
  const total = phase.total_tasks ?? phase.tasks?.length ?? 0;
  const pct   = total > 0 ? Math.round((done / total) * 100) : 0;

  return (
    <div className={`${styles.node} ${styles[phase.status] ?? ''} ${isCurrent ? styles.current : ''}`}>
      <div className={styles.header}>
        <Icon size={15} className={styles.icon} />
        <span className={styles.phaseName}>{phase.name}</span>
        <StatusBadge status={phase.status} size="sm" />
      </div>

      <div className={styles.meta}>
        <span className={styles.taskType}>{phase.task_type}</span>
        <span className={styles.taskCount}>{done}/{total} tasks</span>
      </div>

      <ProgressBar value={pct} size="sm" />

      {phase.tasks && phase.tasks.length > 0 && (
        <div className={styles.taskList}>
          {phase.tasks.map(task => (
            <TaskRow key={task.id} task={task} />
          ))}
        </div>
      )}
    </div>
  );
}
