import { useEffect, useState, useCallback } from 'react';
import DOMPurify from 'dompurify';
import { dashboard, agents, tasks } from '../api/client';

function sanitize(str) {
  if (!str) return '';
  return DOMPurify.sanitize(String(str), { RETURN_TRUSTED_TYPE: false });
}

function refreshAll(setData, setHealth, setTaskList, setError) {
  Promise.all([dashboard.get(), agents.health(), tasks.list()])
    .then(([d, h, t]) => {
      setData(d);
      setHealth(h);
      setTaskList(t.tasks || []);
      setError('');
    })
    .catch(err => setError(err.message));
}

export default function Dashboard() {
  const [data, setData] = useState(null);
  const [health, setHealth] = useState(null);
  const [taskList, setTaskList] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const load = useCallback(() => refreshAll(setData, setHealth, setTaskList, err => { setError(err); setLoading(false); }), []);

  useEffect(() => {
    refreshAll(
      d => { setData(d); setLoading(false); },
      setHealth,
      t => setTaskList(t.tasks || []),
      err => { setError(err.message); setLoading(false); }
    );
    const interval = setInterval(load, 10000);
    return () => clearInterval(interval);
  }, [load]);

  if (loading) return <div className="main"><div style={{ textAlign: 'center', padding: 40, color: 'var(--text-muted)' }}>Loading dashboard...</div></div>;
  if (error) return <div className="main"><p style={{ color: 'var(--danger)' }}>{error}</p><button onClick={load}>Retry</button></div>;

  const taskStats = data?.tasks || {};

  return (
    <div className="main">
      <h2 style={{ marginBottom: 24 }}>Dashboard</h2>

      {/* Agent Health */}
      <div className="stats-grid">
        <div className="stat-card">
          <div className="stat-value" style={{ color: 'var(--success)' }}>{health?.healthy || 0}</div>
          <div className="stat-label">Healthy Agents</div>
        </div>
        <div className="stat-card">
          <div className="stat-value" style={{ color: 'var(--warning)' }}>{health?.warning || 0}</div>
          <div className="stat-label">Warning</div>
        </div>
        <div className="stat-card">
          <div className="stat-value" style={{ color: 'var(--danger)' }}>{health?.dead || 0}</div>
          <div className="stat-label">Dead</div>
        </div>
        <div className="stat-card">
          <div className="stat-value">{taskStats.Total || 0}</div>
          <div className="stat-label">Total Tasks</div>
        </div>
      </div>

      {/* Task Pipeline */}
      <h3 style={{ marginBottom: 16 }}>Task Pipeline</h3>
      <div className="grid-2" style={{ marginBottom: 24 }}>
        <div className="card">
          <h4 className="card-title">Available</h4>
          <p style={{ fontSize: '2rem', fontWeight: 700 }}>{taskStats.Available || 0}</p>
        </div>
        <div className="card">
          <h4 className="card-title">In Progress</h4>
          <p style={{ fontSize: '2rem', fontWeight: 700, color: 'var(--primary)' }}>{taskStats.InProgress || 0}</p>
        </div>
        <div className="card">
          <h4 className="card-title">Review</h4>
          <p style={{ fontSize: '2rem', fontWeight: 700, color: 'var(--warning)' }}>{taskStats.Review || 0}</p>
        </div>
        <div className="card">
          <h4 className="card-title">Test</h4>
          <p style={{ fontSize: '2rem', fontWeight: 700, color: 'var(--success)' }}>{taskStats.Test || 0}</p>
        </div>
        <div className="card">
          <h4 className="card-title">Deployed</h4>
          <p style={{ fontSize: '2rem', fontWeight: 700, color: 'var(--success)' }}>{taskStats.Deployed || 0}</p>
        </div>
        <div className="card">
          <h4 className="card-title">Escalated</h4>
          <p style={{ fontSize: '2rem', fontWeight: 700, color: 'var(--danger)' }}>{taskStats.Escalated || 0}</p>
        </div>
      </div>

      {/* Recent Events */}
      <h3 style={{ marginBottom: 16 }}>Recent Events</h3>
      <div className="card">
        {(data?.events || []).length === 0 ? (
          <p style={{ color: 'var(--text-muted)' }}>No recent events</p>
        ) : (
          data.events.map((ev, i) => (
            <div key={i} className="event-item">
              <p>{sanitize(ev.message || ev.action)}</p>
              <span className="event-time">{ev.created_at}</span>
            </div>
          ))
        )}
      </div>
    </div>
  );
}
