import { type ReactNode } from 'react';
import styles from './Dashboard.module.css';
import { PageHeader } from '../components/shared';
import { StatusBadge } from '../components/shared';
import { ProgressBar } from '../components/shared';
import { apiDashboardSummary } from '../api/client';
import type { DashboardSummary } from '../types';
import { useEffect, useState } from 'react';

function StatCard({ label, value, color }: { label: string; value: number; color: string }) {
  return (
    <div className={styles.stat} style={{ '--c': color } as React.CSSProperties}>
      <span className={styles.statVal}>{value}</span>
      <span className={styles.statLbl}>{label}</span>
    </div>
  );
}

export function Dashboard() {
  const [data, setData] = useState<DashboardSummary | null>(null);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState('');

  useEffect(() => {
    apiDashboardSummary()
      .then(setData)
      .catch(e => setErr(e.message))
      .finally(() => setLoading(false));
  }, []);

  if (loading) return <div className={styles.center}>Loading…</div>;
  if (err)     return <div className={styles.center} style={{ color: 'var(--error)' }}>{err}</div>;
  if (!data)   return null;

  const taskMap = Object.fromEntries(
    (data.task_counts ?? []).map((t: { status: string; count: number }) => [t.status, t.count])
  );
  const total = Object.values(taskMap).reduce((s, v) => s + (v as number), 0) as number;
  const done  = ((taskMap['done'] as number) ?? 0) + ((taskMap['deployed'] as number) ?? 0);
  const pct   = total > 0 ? Math.round((done / total) * 100) : 0;

  return (
    <div>
      <PageHeader title="Dashboard" subtitle="System overview" />
      <div className={styles.statsGrid}>
        <StatCard label="Total tasks"     value={total}                       color="var(--accent)" />
        <StatCard label="Completed"        value={done}                        color="var(--success)" />
        <StatCard label="In progress"    value={(taskMap['in_progress'] as number) ?? 0} color="var(--warning)" />
        <StatCard label="Available"       value={(taskMap['available'] as number) ?? 0} color="var(--info)" />
      </div>
      <div className={styles.grid}>
        <section className={styles.card}>
          <h3 className={styles.cardTitle}>Task distribution</h3>
          {(data.task_counts ?? []).map((t: { status: string; count: number }) => (
            <div key={t.status} className={styles.row}>
              <StatusBadge status={t.status} />
              <span className={styles.rowVal}>{t.count}</span>
            </div>
          ))}
        </section>
        <section className={styles.card}>
          <h3 className={styles.cardTitle}>Overall progress</h3>
          <ProgressBar value={pct} label={`${done} of ${total} tasks done`} />
          <div className={styles.taskList}>
            {(data.recent_tasks ?? []).slice(0, 6).map((t: { id: string; title: string; status: string }) => (
              <div key={t.id} className={styles.row}>
                <span className={styles.taskTitle}>{t.title}</span>
                <StatusBadge status={t.status} size="sm" />
              </div>
            ))}
          </div>
        </section>
        <section className={styles.card}>
          <h3 className={styles.cardTitle}>Active agents</h3>
          {(data.agents ?? []).map((a: { id: string; name: string; status: string; current_tasks: number }) => (
            <div key={a.id} className={styles.row}>
              <span className={styles.dot} data-s={a.status} />
              <span className={styles.agentName}>{a.name}</span>
              <span className={styles.agentTasks}>{a.current_tasks} tasks</span>
              <StatusBadge status={a.status} size="sm" />
            </div>
          ))}
        </section>
      </div>
    </div>
  );
}
