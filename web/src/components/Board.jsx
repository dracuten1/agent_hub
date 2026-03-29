import { useEffect, useState } from 'react';
import DOMPurify from 'dompurify';
import { projects, features } from '../api/client';

function esc(str) {
  if (!str) return '';
  return DOMPurify.sanitize(String(str), { RETURN_TRUSTED_TYPE: false });
}

export default function Board() {
  const [projectList, setProjectList] = useState([]);
  const [selectedProject, setSelectedProject] = useState(null);
  const [featureList, setFeatureList] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const load = () => {
    setLoading(true);
    projects.list()
      .then(data => {
        setProjectList(data.projects || []);
        setLoading(false);
      })
      .catch(err => {
        setError(err.message);
        setLoading(false);
      });
  };

  useEffect(() => {
    load();
    const interval = setInterval(load, 8000);
    return () => clearInterval(interval);
  }, []);

  useEffect(() => {
    if (selectedProject) {
      features.list(selectedProject)
        .then(data => setFeatureList(data.features || []))
        .catch(() => setFeatureList([]));
    } else {
      setFeatureList([]);
    }
  }, [selectedProject]);

  if (loading) return <div className="main"><div style={{ textAlign: 'center', padding: 40, color: 'var(--text-muted)' }}>Loading board...</div></div>;
  if (error) return <div className="main"><p style={{ color: 'var(--danger)', marginBottom: 12 }}>{error}</p><button onClick={load}>Retry</button></div>;

  return (
    <div className="main">
      <h2 style={{ marginBottom: 24 }}>Board</h2>

      {/* Project Tabs */}
      <div className="tabs">
        <button
          className={`tab ${!selectedProject ? 'active' : ''}`}
          onClick={() => setSelectedProject(null)}
        >
          All Projects
        </button>
        {projectList.map(p => (
          <button
            key={p.id}
            className={`tab ${selectedProject === p.id ? 'active' : ''}`}
            onClick={() => setSelectedProject(p.id)}
          >
            {p.name}
          </button>
        ))}
      </div>

      {/* Features (when a project is selected) */}
      {selectedProject && (
        <div style={{ marginBottom: 32 }}>
          <h3 style={{ marginBottom: 16 }}>Features</h3>
          {featureList.length === 0 ? (
            <div className="card" style={{ textAlign: 'center', padding: '24px 0' }}>
              <p style={{ color: 'var(--text-muted)' }}>No features in this project</p>
            </div>
          ) : (
            featureList.map(f => (
              <div key={f.id} className="card" style={{ marginBottom: 12 }}>
                <div className="card-header">
                  <h4>{f.name}</h4>
                  <span className={`badge badge-${f.status === 'active' ? 'success' : 'warning'}`}>{f.status}</span>
                </div>
                {f.description && <p style={{ color: 'var(--text-muted)', fontSize: '0.875rem' }}>{esc(f.description)}</p>}
                <p style={{ fontSize: '0.8rem', color: 'var(--text-muted)', marginTop: 4 }}>Tasks: {f.task_count || 0}</p>
              </div>
            ))
          )}
        </div>
      )}

      {/* Projects */}
      {projectList.length === 0 ? (
        <div className="card" style={{ textAlign: 'center', padding: '40px 0' }}>
          <p style={{ color: 'var(--text-muted)', fontSize: '1.25rem', marginBottom: 8 }}>No projects yet</p>
          <p style={{ color: 'var(--text-muted)', fontSize: '0.875rem' }}>Create a project to get started</p>
        </div>
      ) : (
        projectList
          .filter(p => !selectedProject || p.id === selectedProject)
          .map(project => (
            <div key={project.id} className="card">
              <div className="card-header">
                <h3 className="card-title">{project.name}</h3>
                <span className={`badge badge-${project.status === 'active' ? 'success' : 'warning'}`}>
                  {project.status}
                </span>
              </div>
              {project.description && (
                <p style={{ color: 'var(--text-muted)', marginBottom: 12 }}>{esc(project.description)}</p>
              )}
              <p style={{ fontSize: '0.875rem', color: 'var(--text-muted)' }}>
                Features: {project.feature_count || 0}
              </p>
            </div>
          ))
      )}
    </div>
  );
}
