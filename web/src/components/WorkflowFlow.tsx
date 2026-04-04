import type { PhaseWithTasks } from '../types';
import { PhaseNode } from './PhaseNode';
import styles from './WorkflowFlow.module.css';

// Group consecutive parallel phases ('multi' or 'per_dev') together
interface PhaseGroup {
  phases: PhaseWithTasks[];
  isParallel: boolean;
}

function groupPhases(phases: PhaseWithTasks[]): PhaseGroup[] {
  const groups: PhaseGroup[] = [];
  let i = 0;
  while (i < phases.length) {
    const phase = phases[i];
    const parallel = phase.phase_type === 'multi' || phase.phase_type === 'per_dev';
    if (parallel) {
      // Collect consecutive parallel phases
      const parallelPhases: PhaseWithTasks[] = [phase];
      while (i + parallelPhases.length < phases.length) {
        const next = phases[i + parallelPhases.length];
        if (next.phase_type === 'multi' || next.phase_type === 'per_dev') {
          parallelPhases.push(next);
        } else {
          break;
        }
      }
      groups.push({ phases: parallelPhases, isParallel: true });
      i += parallelPhases.length;
    } else {
      groups.push({ phases: [phase], isParallel: false });
      i++;
    }
  }
  return groups;
}

// ── Parallel fork renderer ───────────────────────────────────────────────────
function ParallelFork({ group, currentPhaseIndex }: {
  group: PhaseGroup;
  currentPhaseIndex: number;
}) {
  const { phases } = group;
  return (
    <div className={styles.fork} style={{ '--lanes': phases.length } as React.CSSProperties}>
      {/* Stem up to fork */}
      <div className={styles.forkStem} />
      {/* Lines spreading to lanes */}
      <div className={styles.forkLines}>
        {phases.map((_, laneIdx) => (
          <div key={laneIdx} className={styles.forkBranch} />
        ))}
      </div>
      {/* Parallel lanes */}
      <div className={styles.lanes}>
        {phases.map((phase, laneIdx) => {
          const isCurrent = phase.index === currentPhaseIndex;
          const pct = phase.total_tasks
            ? Math.round(((phase.completed_tasks ?? 0) / phase.total_tasks) * 100)
            : 0;
          return (
            <div key={phase.id} className={styles.lane}>
              {/* Lane connector */}
              <div className={styles.laneConnector}>
                <div className={`${styles.laneLine} ${phase.status === 'completed' ? styles.laneDone : ''}`} />
              </div>
              {/* Phase card */}
              <div className={`${styles.laneNode} ${isCurrent ? styles.currentLane : ''} ${phase.status === 'completed' ? styles.completedLane : ''}`}>
                <div className={styles.laneHeader}>
                  <span className={styles.laneIdx}>#{laneIdx + 1}</span>
                  <span className={styles.laneName}>{phase.name}</span>
                  {phase.status === 'completed' && (
                    <span className={styles.laneCheck}>✓</span>
                  )}
                </div>
                <div className={styles.laneMeta}>
                  <span>{phase.task_type}</span>
                  <span>{phase.completed_tasks ?? 0}/{phase.total_tasks ?? 0}</span>
                </div>
                {/* Mini progress bar */}
                <div className={styles.miniTrack}>
                  <div
                    className={`${styles.miniFill} ${phase.status === 'completed' ? styles.fillDone : ''}`}
                    style={{ width: `${pct}%` }}
                  />
                </div>
              </div>
            </div>
          );
        })}
      </div>
      {/* Lines merging back */}
      <div className={styles.forkLines}>
        {phases.map((_, laneIdx) => (
          <div key={laneIdx} className={styles.forkBranch} />
        ))}
      </div>
      {/* Stem down from merge */}
      <div className={styles.forkStem} />
    </div>
  );
}

// ── Main export ──────────────────────────────────────────────────────────────
export function WorkflowFlow({ phases, currentPhaseIndex }: {
  phases: PhaseWithTasks[];
  currentPhaseIndex: number;
}) {
  const groups = groupPhases(phases);

  return (
    <div className={styles.flow}>
      {groups.map((group, gi) => {
        if (group.isParallel) {
          return (
            <div key={gi} className={styles.group}>
              {gi > 0 && <div className={`${styles.connector} ${groups[gi - 1].phases.at(-1)?.status === 'completed' ? styles.done : ''}`} />}
              <ParallelFork group={group} currentPhaseIndex={currentPhaseIndex} />
              {gi < groups.length - 1 && (
                <div className={`${styles.connector} ${group.phases.at(-1)?.status === 'completed' ? styles.done : ''}`} />
              )}
            </div>
          );
        }

        // Single/sequential phase
        const phase = group.phases[0];
        return (
          <div key={gi} className={styles.group}>
            <PhaseNode phase={phase} isCurrent={phase.index === currentPhaseIndex} />
            {gi < groups.length - 1 && (
              <div className={`${styles.connector} ${phase.status === 'completed' ? styles.done : ''}`} />
            )}
          </div>
        );
      })}
    </div>
  );
}
