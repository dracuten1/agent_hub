import { Link, useLocation } from 'react-router-dom';
import { LayoutDashboard, GitBranch, FolderKanban, Bot } from 'lucide-react';
import styles from './BottomNav.module.css';

const nav = [
  { to: '/',         icon: LayoutDashboard, label: 'Home' },
  { to: '/workflows', icon: GitBranch,        label: 'Flows' },
  { to: '/projects', icon: FolderKanban,     label: 'Projects' },
  { to: '/agents',   icon: Bot,              label: 'Agents' },
];

export function BottomNav() {
  const { pathname } = useLocation();
  return (
    <nav className={styles.nav}>
      {nav.map(({ to, icon: Icon, label }) => {
        const active = to === '/' ? pathname === '/' : pathname.startsWith(to);
        return (
          <Link key={to} to={to} className={`${styles.tab} ${active ? styles.active : ''}`}>
            <Icon size={20} />
            <span>{label}</span>
          </Link>
        );
      })}
    </nav>
  );
}
