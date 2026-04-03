import { Link } from 'react-router-dom';
import type { Project } from '../types';
import { StatusBadge } from './shared';
import styles from './ProjectCard.module.css';

interface Props {
  project: Project;
}

export function ProjectCard({ project }: Props) {
  return (
    <Link to={`/projects/${project.id}`} className={styles.card}>
      <div className={styles.top}>
        <span className={styles.name}>{project.name}</span>
        <StatusBadge status={project.status} size="sm" />
      </div>
      {project.description && <p className={styles.desc}>{project.description}</p>}
      <div className={styles.meta}>
        {project.task_count !== undefined && <span>{project.task_count} tasks</span>}
        <span>{new Date(project.created_at).toLocaleDateString()}</span>
      </div>
    </Link>
  );
}
