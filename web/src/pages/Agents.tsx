import { useEffect, useState } from 'react';
import { Activity, Cpu, CheckCircle, XCircle } from 'lucide-react';
import { PageHeader, StatusBadge } from '../components/shared';
import { apiAgents } from '../api/client';
import type { Agent } from '../types';
import styles from './Agents.module.css';

export function Agents() {
  const [agents, setAgents] = useState<Agent[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    apiAgents().then(r => setAgents(r.agents)).finally(() => setLoading(false));
  }, []);

  const healthy = agents.filter(a => a.status === 'idle' || a.status === 'busy').length;
  const dead    = agents.filter(a => a.status === 'offline' || a.status === 'error').length;

  return (
    <div>
      <PageHeader title="Agents" subtitle={`${agents.length} registered`} />
      <div className={styles.healthRow}>
        <div className={`${styles.health} ${styles.ok}`}>
          <CheckCircle size={16} /><span><strong>{healthy}</strong> healthy</span>
        </div>
        <div className={`${styles.health} ${styles.dead}`}>
          <XCircle size={16} /><span><strong>{dead}</strong> offline</span>
        </div>
      </div>
      {loading ? <div className={styles.center}>Loading…</div>
       : (
        <div className={styles.grid}>
          {agents.map(a => (
            <div key={a.id} className={styles.card}>
              <div className={styles.top}>
                <div className={styles.icon}><Cpu size={16} /></div>
                <div className={styles.info}>
                  <span className={styles.name}>{a.name}</span>
                  <span className={styles.role}>{a.role}</span>
                </div>
                <StatusBadge status={a.status} />
              </div>
              <div className={styles.stats}>
                <div className={styles.stat}><Activity size={11} /><span>{a.current_tasks}/{a.max_tasks} tasks</span></div>
                <div className={styles.stat}><CheckCircle size={11} /><span>{a.total_completed} done</span></div>
                <div className={styles.stat}><XCircle size={11} /><span>{a.total_failed} failed</span></div>
              </div>
              {a.model && <div className={styles.meta}>model: {a.model}</div>}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
