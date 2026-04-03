import { type FormEvent, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { Plus } from 'lucide-react';
import { PageHeader, StatusBadge, ProgressBar } from '../components/shared';
import { apiWorkflows, apiWorkflowTemplates, apiStartWorkflow } from '../api/client';
import type { Workflow, WorkflowTemplate } from '../types';
import styles from './Workflows.module.css';

function WorkflowRow({ wf }: { wf: Workflow }) {
  return (
    <Link to={`/workflows/${wf.id}`} className={styles.row}>
      <div className={styles.rowLeft}>
        <span className={styles.wfName}>{wf.name}</span>
        <span className={styles.wfMeta}>Phase {wf.current_phase}/{wf.total_phases}</span>
      </div>
      <div className={styles.rowRight}>
        {wf.progress !== undefined && wf.progress > 0 && (
          <div className={styles.mini}>
            <ProgressBar value={wf.progress} size="sm" />
          </div>
        )}
        <StatusBadge status={wf.status} size="sm" />
      </div>
    </Link>
  );
}

export function Workflows() {
  const [workflows, setWorkflows] = useState<Workflow[]>([]);
  const [loading, setLoading] = useState(true);
  const [showCreate, setShowCreate] = useState(false);
  const [templates, setTemplates] = useState<WorkflowTemplate[]>([]);
  const [createName, setCreateName] = useState('');
  const [createTemplate, setCreateTemplate] = useState('');
  const [createLoading, setCreateLoading] = useState(false);
  const [createError, setCreateError] = useState('');

  useEffect(() => {
    apiWorkflows().then(r => setWorkflows(r.workflows)).finally(() => setLoading(false));
    apiWorkflowTemplates().then(r => setTemplates(r.templates)).catch(() => {});
  }, []);

  async function handleCreate(e: FormEvent) {
    e.preventDefault();
    setCreateError('');
    setCreateLoading(true);
    try {
      const { workflow } = await apiStartWorkflow({ name: createName, template_id: createTemplate || undefined });
      setWorkflows(prev => [workflow, ...prev]);
      setShowCreate(false);
      setCreateName('');
    } catch (err) {
      setCreateError(err instanceof Error ? err.message : 'Failed');
    } finally {
      setCreateLoading(false);
    }
  }

  return (
    <div>
      <PageHeader
        title="Workflows"
        subtitle={`${workflows.length} workflow${workflows.length !== 1 ? 's' : ''}`}
        actions={
          <button className={styles.btn} onClick={() => setShowCreate(s => !s)}>
            <Plus size={16} /> New workflow
          </button>
        }
      />
      {showCreate && (
        <form onSubmit={handleCreate} className={styles.createForm}>
          <div className={styles.formRow}>
            <input className={styles.input} type="text" placeholder="Workflow name" required
              value={createName} onChange={e => setCreateName(e.target.value)} />
            <select className={styles.select} value={createTemplate} onChange={e => setCreateTemplate(e.target.value)}>
              <option value="">— select template —</option>
              {templates.map(t => <option key={t.id} value={t.id}>{t.name}</option>)}
            </select>
            <button type="submit" className={styles.btn} disabled={createLoading}>
              {createLoading ? 'Starting…' : 'Start'}
            </button>
            <button type="button" className={styles.cancelBtn} onClick={() => setShowCreate(false)}>Cancel</button>
          </div>
          {createError && <p className={styles.error}>{createError}</p>}
        </form>
      )}
      {loading ? <div className={styles.center}>Loading…</div>
       : workflows.length === 0 ? (
        <div className={styles.center}>
          <p>No workflows yet.</p>
          <button className={styles.btn} onClick={() => setShowCreate(true)}>Start your first workflow</button>
        </div>
      ) : (
        <div className={styles.list}>
          {workflows.map(wf => <WorkflowRow key={wf.id} wf={wf} />)}
        </div>
      )}
    </div>
  );
}
