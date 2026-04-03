import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { PageHeader, StatusBadge } from '../components/shared';
import { apiProjects } from '../api/client';
import type { Project } from '../types';
import styles from './Projects.module.css';

export function Projects() {
  const [projects, setProjects] = useState<Project[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    apiProjects().then(r => setProjects(r.projects)).finally(() => setLoading(false));
  }, []);

  return (
    <div>
      <PageHeader title="Projects" subtitle={`${projects.length} project${projects.length !== 1 ? 's' : ''}`} />
      {loading ? <div className={styles.center}>Loading…</div>
       : projects.length === 0 ? <div className={styles.center}><p>No projects yet.</p></div>
       : (
        <div className={styles.grid}>
          {projects.map(p => (
            <Link key={p.id} to={`/projects/${p.id}`} className={styles.card}>
              <div className={styles.top}>
                <span className={styles.name}>{p.name}</span>
                <StatusBadge status={p.status} size="sm" />
              </div>
              {p.description && <p className={styles.desc}>{p.description}</p>}
              <div className={styles.meta}>
                {p.task_count !== undefined && <span>{p.task_count} tasks</span>}
                <span>{new Date(p.created_at).toLocaleDateString()}</span>
              </div>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
