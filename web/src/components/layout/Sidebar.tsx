import { Link, useLocation } from 'react-router-dom';
import { LayoutDashboard, GitBranch, FolderKanban, Bot } from 'lucide-react';
import styles from './Sidebar.module.css';

const nav = [
  { to: '/',          icon: LayoutDashboard, label: 'Dashboard' },
  { to: '/workflows',  icon: GitBranch,        label: 'Workflows' },
  { to: '/projects',   icon: FolderKanban,     label: 'Projects' },
  { to: '/agents',    icon: Bot,               label: 'Agents' },
];

function logout() {
  localStorage.removeItem('token');
  window.location.href = '/login';
}

export function Sidebar() {
  const { pathname } = useLocation();
  return (
    <nav className={styles.sidebar}>
      <div className={styles.brand}>
        <span className={styles.brandMark}>A</span>
        <span className={styles.brandText}>AgentHub</span>
      </div>

      <ul className={styles.nav}>
        {nav.map(({ to, icon: Icon, label }) => {
          const active = to === '/' ? pathname === '/' : pathname.startsWith(to);
          return (
            <li key={to}>
              <Link to={to} className={`${styles.link} ${active ? styles.active : ''}`}>
                <Icon size={18} />
                <span>{label}</span>
              </Link>
            </li>
          );
        })}
      </ul>

      <button className={styles.logout} onClick={logout}>
        Sign out
      </button>
    </nav>
  );
}
