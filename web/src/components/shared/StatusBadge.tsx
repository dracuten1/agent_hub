import styles from './StatusBadge.module.css';

interface Props {
  status: string;
  size?: 'sm' | 'md';
}

const map: Record<string, string> = {
  // Workflow
  running:     'accent',
  completed:  'success',
  failed:     'error',
  cancelled:  'muted',
  paused:     'warning',
  // Phase
  pending:    'muted',
  skipped:    'muted',
  waiting_approval: 'warning',
  // Task
  available:  'info',
  in_progress:'accent',
  done:       'success',
  deployed:   'success',
  blocked:    'error',
  // Agent
  idle:       'success',
  busy:       'accent',
  offline:    'muted',
  error:      'error',
  // Generic fallback
  active:     'accent',
  inactive:   'muted',
};

export function StatusBadge({ status, size = 'md' }: Props) {
  const variant = map[status] ?? 'muted';
  return (
    <span className={`${styles.badge} ${styles[variant]} ${size === 'sm' ? styles.sm : ''}`}>
      {status.replace(/_/g, ' ')}
    </span>
  );
}
