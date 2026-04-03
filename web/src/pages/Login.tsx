import { type FormEvent, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { apiLogin } from '../api/client';
import styles from './Login.module.css';

export function Login() {
  const navigate = useNavigate();
  const [fields, setFields] = useState({ username: '', password: '' });
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  function set(k: string, v: string) { setFields(f => ({ ...f, [k]: v })); }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError('');
    setLoading(true);
    try {
      const { token } = await apiLogin(fields.username, fields.password);
      localStorage.setItem('token', token);
      navigate('/', { replace: true });
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Login failed');
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className={styles.page}>
      <div className={styles.card}>
        <div className={styles.logo}>
          <span className={styles.logoMark}>A</span>
          <span className={styles.logoText}>AgentHub</span>
        </div>
        <h2 className={styles.heading}>Sign in</h2>
        <p className={styles.sub}>Enter your credentials to continue.</p>
        <form onSubmit={handleSubmit} className={styles.form}>
          <div className={styles.field}>
            <label htmlFor="username">Username</label>
            <input id="username" type="text" autoComplete="username"
              value={fields.username} onChange={e => set('username', e.target.value)}
              placeholder="admin" required />
          </div>
          <div className={styles.field}>
            <label htmlFor="password">Password</label>
            <input id="password" type="password" autoComplete="current-password"
              value={fields.password} onChange={e => set('password', e.target.value)}
              placeholder="••••••••" required />
          </div>
          {error && <p className={styles.error}>{error}</p>}
          <button type="submit" className={styles.btn} disabled={loading}>
            {loading ? 'Signing in…' : 'Sign in'}
          </button>
        </form>
      </div>
    </div>
  );
}
