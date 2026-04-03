import { type FC, useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { ArrowLeft } from 'lucide-react';
import { WorkflowFlow } from '../components/WorkflowFlow';
import { PhaseNode } from '../components/PhaseNode';
import { PageHeader, StatusBadge, ProgressBar } from '../components/shared';
import { apiWorkflow, apiApproveWorkflow } from '../api/client';
import type { WorkflowDetailResponse } from '../types';
import styles from './WorkflowDetail.module.css';

export function WorkflowDetail() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [data, setData] = useState<WorkflowDetailResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [approveLoading, setApproveLoading] = useState(false);

  function load() {
    if (!id) return;
    apiWorkflow(id).then(setData).catch(() => navigate('/workflows')).finally(() => setLoading(false));
  }

  useEffect(() => { load(); }, [id]);

  async function handleApprove() {
    if (!id) return;
    setApproveLoading(true);
    try { await apiApproveWorkflow(id); load(); }
    finally { setApproveLoading(false); }
  }

  if (loading) return <div className={styles.center}>Loading…</div>;
  if (!data) return null;

  const { workflow, phases, progress } = data;

  return (
    <div>
      <PageHeader
        title={workflow.name}
        subtitle={`Phase ${workflow.current_phase} of ${workflow.total_phases}`}
        actions={<button className={styles.backBtn} onClick={() => navigate('/workflows')}><ArrowLeft size={16} /> Back</button>}
      />
      <div className={styles.summaryBar}>
        <StatusBadge status={workflow.status} />
        <ProgressBar value={progress.percentage} label="Overall progress" />
      </div>
      <div className={styles.pipelineScroll}>
        <WorkflowFlow phases={phases} currentPhaseIndex={workflow.current_phase} />
      </div>
      {workflow.status === 'paused' && (
        <div className={styles.approveBar}>
          <p>This workflow is paused waiting for approval.</p>
          <button className={styles.approveBtn} onClick={handleApprove} disabled={approveLoading}>
            {approveLoading ? 'Approving…' : 'Approve & advance'}
          </button>
        </div>
      )}
    </div>
  );
}
