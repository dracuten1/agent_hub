import { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { ArrowLeft } from 'lucide-react';
import { PageHeader, StatusBadge, ProgressBar } from '../components/shared';
import { apiProject } from '../api/client';
import type { ProjectDetailResponse } from '../types';
import styles from './ProjectDetail.module.css';

export function ProjectDetail() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [data, setData] = useState<ProjectDetailResponse | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (!id) return;
    apiProject(id).then(setData).catch(() => navigate('/projects')).finally(() => setLoading(false));
  }, [id]);

  if (loading) return <div className={styles.center}>Loading…</div>;
  if (!data) return null;

  const { project, features, stats } = data;
  const pct = stats.completion_rate ?? 0;

  return (
    <div>
      <PageHeader
        title={project.name}
        subtitle={project.description || project.id}
        actions={<button className={styles.backBtn} onClick={() => navigate('/projects')}><ArrowLeft size={16} /> Back</button>}
      />
      <div className={styles.summaryBar}>
        <StatusBadge status={project.status} />
        <ProgressBar value={pct} label="Completion" color="success" />
        <span className={styles.stat}>{stats.total_tasks} tasks</span>
      </div>
      <section>
        <h2 className={styles.sectionTitle}>Features ({features.length})</h2>
        <div className={styles.list}>
          {features.map(f => (
            <div key={f.id} className={styles.row}>
              <div className={styles.rowLeft}>
                <StatusBadge status={f.status} size="sm" />
                <span className={styles.featureName}>{f.name}</span>
              </div>
              <div className={styles.rowRight}>
                <span className={styles.featureStats}>{f.completed_tasks}/{f.task_count} tasks</span>
                {f.task_count > 0 && (
                  <ProgressBar value={Math.round((f.completed_tasks / f.task_count) * 100)} size="sm" color="success" />
                )}
              </div>
            </div>
          ))}
        </div>
      </section>
    </div>
  );
}
