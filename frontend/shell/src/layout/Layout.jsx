import React from 'react';
import { NavLink, Outlet, useNavigate, useSearchParams } from 'react-router-dom';
import { bus } from '../shared';
import Logo from './Logo';

const NAV_ITEMS = [
  { to: '/home', label: '首页', icon: '🏠', disabled: false },
  { to: '/explore', label: '探索', icon: '🔍', disabled: false },
  { to: '/notifications', label: '通知', icon: '🔔', disabled: true, hint: 'P4 开放' },
  { to: '/profile', label: '我的', icon: '👤', disabled: true, hint: 'P2 开放' },
];

/**
 * Layout — 3 columns:
 * left nav 260px (logo + nav links + logout) | middle <Outlet/> | right rail 320px (search).
 * Right rail hidden ≤1024px.
 */
export default function Layout() {
  const navigate = useNavigate();
  const [params] = useSearchParams();

  const handleSearch = (e) => {
    e.preventDefault();
    const q = e.target.q.value.trim();
    if (q) navigate(`/explore?q=${encodeURIComponent(q)}`);
  };

  const handleLogout = () => {
    localStorage.removeItem('b_token');
    bus.emit(bus.events.AUTH_LOGOUT);
    navigate('/login', { replace: true });
  };

  return (
    <div className="layout">
      <aside className="layout__left">
        <div className="layout__logo-row">
          <Logo />
          <span className="layout__brand">B</span>
        </div>
        <nav className="layout__nav">
          {NAV_ITEMS.map((item) =>
            item.disabled ? (
              <span key={item.to} className="nav-item disabled" title={item.hint}>
                <span className="nav-item__icon">{item.icon}</span>
                {item.label}
              </span>
            ) : (
              <NavLink
                key={item.to}
                to={item.to}
                className={({ isActive }) => `nav-item${isActive ? ' active' : ''}`}
              >
                <span className="nav-item__icon">{item.icon}</span>
                {item.label}
              </NavLink>
            )
          )}
        </nav>
        <div className="layout__spacer" />
        <button type="button" className="btn btn--ghost layout__logout" onClick={handleLogout}>
          退出登录
        </button>
      </aside>

      <main className="layout__main">
        <Outlet />
      </main>

      <aside className="layout__right">
        <form className="layout__search" onSubmit={handleSearch}>
          <span className="layout__search-icon">🔍</span>
          <input
            className="input"
            name="q"
            type="search"
            placeholder="搜索帖子…"
            defaultValue={params.get('q') || ''}
            aria-label="搜索"
          />
        </form>
        <div className="layout__right-card">
          <h4>关于 B</h4>
          <p>简化版 社交平台 — 微服务学习演示项目。</p>
        </div>
      </aside>
    </div>
  );
}
