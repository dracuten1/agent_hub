import { CheckCircle, Clock, XCircle, Circle } from 'lucide-react';
import type { PhaseWithTasks } from '../types';
import { TaskList } from './TaskList';
import { StatusBadge, ProgressBar } from './shared';
import styles from './PhaseNode.module.css';

interface Props {
  phase: PhaseWithTasks;
  isCurrent: boolean;
}

const icons: Record<string, React.FC<{ size?: number; className?: string }>> = {
  completed:  CheckCircle,
  running:    Clock,
  failed:     XCircle,
};

export function PhaseNode({ phase, isCurrent }: Props) {
  const Icon = icons[phase.status] ?? Circle;
  const done   = phase.completed_tasks ?? 0;
  const total  = phase.total_tasks ?? phase.tasks?.length ?? 0;
  const pct    = total > 0 ? Math.round((done / total) * 100) : 0;

  return (
    <div className={`${styles.node} ${styles[phase.status] ?? ''} ${isCurrent ? styles.current : ''}`}>
      <div className={styles.header}>
        <Icon size={16} className={styles.icon} />
        <span className={styles.name}>{phase.name}</span>
        <StatusBadge status={phase.status} size="sm" />
      </div>
      <div className={styles.meta}>
        <span>{phase.task_type}</span>
        <span>{done}/{total} tasks</span>
      </div>
      <ProgressBar value={pct} size="sm" />
      {phase.tasks && phase.tasks.length > 0 && (
        <TaskList tasks={phase.tasks} />
      )}
    </div>
  );
}
