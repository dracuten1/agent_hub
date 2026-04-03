import { Cpu, Activity } from 'lucide-react';
import type { Agent } from '../types';
import { StatusBadge } from './shared';
import styles from './AgentCard.module.css';

interface Props {
  agent: Agent;
}

export function AgentCard({ agent }: Props) {
  return (
    <div className={styles.card}>
      <div className={styles.top}>
        <div className={styles.icon}><Cpu size={18} /></div>
        <div className={styles.info}>
          <span className={styles.name}>{agent.name}</span>
          <span className={styles.role}>{agent.role}</span>
        </div>
        <StatusBadge status={agent.status} />
      </div>
      <div className={styles.stats}>
        <div className={styles.stat}>
          <Activity size={11} />
          <span>{agent.current_tasks}/{agent.max_tasks} tasks</span>
        </div>
        <div className={styles.stat}>
          <Activity size={11} />
          <span>{agent.total_completed} done · {agent.total_failed} failed</span>
        </div>
      </div>
      {agent.model && <div className={styles.meta}>model: {agent.model}</div>}
    </div>
  );
}
