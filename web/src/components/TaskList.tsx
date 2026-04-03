import type { Task } from '../types';
import { StatusBadge } from './shared';
import styles from './TaskList.module.css';

interface Props {
  tasks: Task[];
  maxVisible?: number;
}

export function TaskList({ tasks, maxVisible = 4 }: Props) {
  const visible = tasks.slice(0, maxVisible);
  const hidden  = tasks.length - maxVisible;

  return (
    <div className={styles.list}>
      {visible.map(task => (
        <div key={task.id} className={styles.item}>
          <StatusBadge status={task.status} size="sm" />
          <span className={styles.title}>{task.title}</span>
          {task.assignee && <span className={styles.assignee}>{task.assignee}</span>}
        </div>
      ))}
      {hidden > 0 && (
        <div className={styles.more}>+{hidden} more</div>
      )}
    </div>
  );
}
