import type { PhaseWithTasks } from '../types';
import { PhaseNode } from './PhaseNode';
import styles from './WorkflowFlow.module.css';

// ── Helpers ────────────────────────────────────────────────────────────────────

function getSubtype(phase: PhaseWithTasks): string {
  const cfg: Record<string, unknown> =
    typeof phase.config === 'object' && phase.config !== null ? phase.config : {};
  if (phase.phase_type === 'gate') {
    if (cfg.require_owner || cfg.approver === 'owner') return 'human_gate';
    return 'agent_gate';
  }
  return phase.phase_type ?? 'normal';
}

function SubtypeBadgeInline({ subtype }: { subtype: string }) {
  const colors: Record<string, string> = {
    agent_gate: '#60a5fa',
    human_gate: '#fb923c',
    decision:   '#c084fc',
    per_dev:    '#34d399',
  };
  const color = colors[subtype];
  if (!color) return null;
  return (
    <span style={{
      fontSize: 9, fontWeight: 600, color, background: `${color}22`,
      padding: '1px 5px', borderRadius: 4, textTransform: 'uppercase', letterSpacing: 0.3
    }}>
      {subtype.replace('_', ' ')}
    </span>
  );
}

// ── Phase lane node (simplified for parallel lanes) ─────────────────────────────

function PhaseLaneMini({ phase, isCurrent }: { phase: PhaseWithTasks; isCurrent: boolean }) {
  const pct = phase.total_tasks
    ? Math.round(((phase.completed_tasks ?? 0) / phase.total_tasks) * 100)
    : 0;
  const subtype = getSubtype(phase);

  return (
    <>
      <div className={styles.laneHeader}>
        <span className={styles.laneIdx}>#{phase.index}</span>
        <span className={styles.laneName}>{phase.name}</span>
        <SubtypeBadgeInline subtype={subtype} />
        {phase.status === 'completed' && <span className={styles.laneCheck}>✓</span>}
      </div>
      <div className={styles.laneMeta}>
        <span>{phase.task_type}</span>
        <span>{phase.completed_tasks ?? 0}/{phase.total_tasks ?? 0}</span>
      </div>
      <div className={styles.miniTrack}>
        <div
          className={`${styles.miniFill} ${phase.status === 'completed' ? styles.fillDone : ''}`}
          style={{ width: `${pct}%` }}
        />
      </div>
    </>
  );
}

// ── Group consecutive parallel phases ─────────────────────────────────────────

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

// ── Parallel fork renderer ─────────────────────────────────────────────────────

function ParallelFork({ group, currentPhaseIndex }: {
  group: PhaseGroup;
  currentPhaseIndex: number;
}) {
  const { phases } = group;
  return (
    <div className={styles.fork} style={{ '--lanes': phases.length } as React.CSSProperties}>
      {/* Stem */}
      <div className={styles.forkStem} />
      {/* Spreading lines */}
      <div className={styles.forkLines}>
        {phases.map((_, laneIdx) => (
          <div key={laneIdx} className={styles.forkBranch} />
        ))}
      </div>
      {/* Parallel lanes */}
      <div className={styles.lanes}>
        {phases.map((phase, laneIdx) => {
          const isCurrent = phase.index === currentPhaseIndex;
          return (
            <div key={phase.id} className={styles.lane}>
              <div className={styles.laneConnector}>
                <div className={`${styles.laneLine} ${phase.status === 'completed' ? styles.laneDone : ''}`} />
              </div>
              <div className={`${styles.laneNode} ${isCurrent ? styles.currentLane : ''} ${phase.status === 'completed' ? styles.completedLane : ''}`}>
                <PhaseLaneMini phase={phase} isCurrent={isCurrent} />
              </div>
            </div>
          );
        })}
      </div>
      {/* Merging lines */}
      <div className={styles.forkLines}>
        {phases.map((_, laneIdx) => (
          <div key={laneIdx} className={styles.forkBranch} />
        ))}
      </div>
      {/* Stem down */}
      <div className={styles.forkStem} />
    </div>
  );
}

// ── Main export ────────────────────────────────────────────────────────────────

export function WorkflowFlow({ phases, currentPhaseIndex, workflowId }: {
  phases: PhaseWithTasks[];
  currentPhaseIndex: number;
  workflowId?: string;
}) {
  const groups = groupPhases(phases);

  return (
    <div className={styles.flow}>
      {groups.map((group, gi) => {
        if (group.isParallel) {
          return (
            <div key={gi} className={styles.group}>
              {gi > 0 && (
                <div className={`${styles.connector} ${groups[gi - 1].phases.at(-1)?.status === 'completed' ? styles.done : ''}`} />
              )}
              <ParallelFork group={group} currentPhaseIndex={currentPhaseIndex} />
              {gi < groups.length - 1 && (
                <div className={`${styles.connector} ${group.phases.at(-1)?.status === 'completed' ? styles.done : ''}`} />
              )}
            </div>
          );
        }

        const phase = group.phases[0];
        return (
          <div key={gi} className={styles.group}>
            <PhaseNode phase={phase} isCurrent={phase.index === currentPhaseIndex} workflowId={workflowId} />
            {gi < groups.length - 1 && (
              <div className={`${styles.connector} ${phase.status === 'completed' ? styles.done : ''}`} />
            )}
          </div>
        );
      })}
    </div>
  );
}
