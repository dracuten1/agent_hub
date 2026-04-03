import type { PhaseWithTasks } from '../types';
import { PhaseNode } from './PhaseNode';
import styles from './WorkflowFlow.module.css';

interface Props {
  phases: PhaseWithTasks[];
  currentPhaseIndex: number; // 1-based
}

export function WorkflowFlow({ phases, currentPhaseIndex }: Props) {
  return (
    <div className={styles.flow}>
      {phases.map((phase, i) => (
        <div key={phase.id} className={styles.step}>
          <PhaseNode phase={phase} isCurrent={phase.index === currentPhaseIndex} />
          {i < phases.length - 1 && (
            <div className={`${styles.connector} ${phase.status === 'completed' ? styles.done : ''}`} />
          )}
        </div>
      ))}
    </div>
  );
}
