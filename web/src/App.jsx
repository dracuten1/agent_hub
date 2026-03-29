import { useState, useEffect } from 'react';
import { getToken, setToken } from './api/client';
import Login from './components/Login';
import Dashboard from './components/Dashboard';
import Board from './components/Board';
import './index.css';

function App() {
  const [user, setUser] = useState(null);
  const [tab, setTab] = useState('dashboard');
  const [authChecked, setAuthChecked] = useState(false);

  useEffect(() => {
    const t = getToken();
    if (t) {
      try {
        const payload = JSON.parse(atob(t.split('.')[1]));
        if (payload.exp * 1000 > Date.now()) {
          setUser({ username: payload.username, id: payload.sub });
        } else {
          setToken(null);
        }
      } catch {
        setToken(null);
      }
    }
    setAuthChecked(true);
  }, []);

  const handleLogout = () => {
    setToken(null);
    setUser(null);
  };

  if (!authChecked) {
    return (
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100vh' }}>
        <div style={{ textAlign: 'center' }}>
          <div style={{ fontSize: 32, marginBottom: 8 }}>🤖</div>
          <div style={{ color: 'var(--text-muted)', fontSize: '0.875rem' }}>Loading AgentHub...</div>
        </div>
      </div>
    );
  }

  if (!user) {
    return <Login onLogin={setUser} />;
  }

  return (
    <div className="app">
      <nav className="navbar">
        <div className="navbar-brand">🤖 AgentHub</div>
        <div style={{ display: 'flex', gap: 16 }}>
          <button className={`tab ${tab === 'dashboard' ? 'active' : ''}`} onClick={() => setTab('dashboard')}>
            Dashboard
          </button>
          <button className={`tab ${tab === 'board' ? 'active' : ''}`} onClick={() => setTab('board')}>
            Board
          </button>
        </div>
        <div>
          <span className="navbar-user">{user.username}</span>
          <button className="btn-logout" onClick={handleLogout}>Logout</button>
        </div>
      </nav>
      {tab === 'dashboard' ? <Dashboard /> : <Board />}
    </div>
  );
}

export default App;
