import styles from './ProgressBar.module.css';

interface Props {
  value: number;
  label?: string;
  size?: 'sm' | 'md';
  color?: 'accent' | 'success' | 'warning';
}

export function ProgressBar({ value, label, size = 'md', color = 'accent' }: Props) {
  const pct = Math.min(100, Math.max(0, value));
  return (
    <div className={styles.wrapper}>
      {label && (
        <span className={styles.label}>
          {label} <strong>{pct}%</strong>
        </span>
      )}
      <div className={`${styles.track} ${size === 'sm' ? styles.sm : ''}`}>
        <div className={`${styles.fill} ${styles[color]}`} style={{ width: `${pct}%` }} />
      </div>
    </div>
  );
}
