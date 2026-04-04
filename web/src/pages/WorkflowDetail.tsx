import { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { ArrowLeft } from 'lucide-react';
import { WorkflowFlow } from '../components/WorkflowFlow';
import { GateApprovalBar } from '../components/GateApprovalBar';
import { PageHeader, StatusBadge, ProgressBar } from '../components/shared';
import { apiWorkflow, apiApproveWorkflow, apiRejectWorkflow } from '../api/client';
import type { PhaseWithTasks, WorkflowDetailResponse } from '../types';
import styles from './WorkflowDetail.module.css';

// Find the first paused gate/decision phase that needs approval
function findGatePhase(phases: PhaseWithTasks[]): PhaseWithTasks | null {
  return phases.find(p => p.status === 'paused' && (p.phase_type === 'gate' || p.phase_type === 'decision')) ?? null;
}

// Get the last completed phase before the gate
function getPreviousPhase(phases: PhaseWithTasks[], gateIndex: number) {
  for (let i = gateIndex - 1; i >= 0; i--) {
    const p = phases[i];
    if (p.status === 'completed' && p.tasks && p.tasks.length > 0) {
      const done = p.completed_tasks ?? 0;
      const total = p.total_tasks ?? p.tasks.length;
      return {
        name: p.name,
        tasks: p.tasks.map(t => ({
          title: t.title,
          status: t.status,
          result: t.result,
          assignee: t.assignee,
        })),
        total,
        done,
      };
    }
  }
  return null;
}

export function WorkflowDetail() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [data, setData] = useState<WorkflowDetailResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [approveLoading, setApproveLoading] = useState(false);
  const [rejectLoading, setRejectLoading] = useState(false);
  const [gateError, setGateError] = useState('');

  function load() {
    if (!id) return;
    apiWorkflow(id)
      .then(setData)
      .catch(() => navigate('/workflows'))
      .finally(() => setLoading(false));
  }

  useEffect(() => { load(); }, [id]);

  async function handleApprove(note: string) {
    if (!id) return;
    setGateError('');
    setApproveLoading(true);
    try {
      await apiApproveWorkflow(id, note || undefined);
      load();
    } catch (err) {
      setGateError(err instanceof Error ? err.message : 'Approval failed');
    } finally {
      setApproveLoading(false);
    }
  }

  async function handleReject(note: string) {
    if (!id) return;
    setGateError('');
    setRejectLoading(true);
    try {
      await apiRejectWorkflow(id, note || undefined);
      load();
    } catch (err) {
      setGateError(err instanceof Error ? err.message : 'Reject failed — backend may not support this');
    } finally {
      setRejectLoading(false);
    }
  }

  if (loading) return <div className={styles.center}>Loading…</div>;
  if (!data) return null;

  const { workflow, phases, progress } = data;
  const gatePhase = workflow.status === 'paused' ? findGatePhase(phases) : null;
  const gateIdx   = gatePhase ? phases.indexOf(gatePhase) : -1;
  const prevPhase = gateIdx > 0 ? getPreviousPhase(phases, gateIdx) : null;

  return (
    <div>
      <PageHeader
        title={workflow.name}
        subtitle={`Phase ${workflow.current_phase} of ${workflow.total_phases}`}
        actions={
          <button className={styles.backBtn} onClick={() => navigate('/workflows')}>
            <ArrowLeft size={16} /> Back
          </button>
        }
      />

      <div className={styles.summaryBar}>
        <StatusBadge status={workflow.status} />
        <ProgressBar value={progress.percentage} label="Overall progress" />
      </div>

      <div className={styles.pipelineScroll}>
        <WorkflowFlow phases={phases} currentPhaseIndex={workflow.current_phase} />
      </div>

      {gatePhase && (
        <GateApprovalBar
          gatePhaseIndex={gateIdx + 1}
          previousPhase={prevPhase}
          onApprove={handleApprove}
          onReject={handleReject}
          approveLoading={approveLoading}
          rejectLoading={rejectLoading}
          error={gateError}
        />
      )}
    </div>
  );
}
