import { createRoot } from 'react-dom/client';
import { BrowserRouter, Link, Navigate, Route, Routes, useNavigate } from 'react-router-dom';
import Auth from '../src/Auth.jsx';
import Profile from '../src/Profile.jsx';
import { api, bus } from '@b/shared';
import '../src/styles.css';

/**
 * Standalone dev shell — NOT part of the Module Federation build.
 * Bundles the exposed components with a local @b/shared shim
 * (webpack.standalone.js aliases '@b/shared' → dev/shared-shim.js) so the UI
 * can be verified without the Shell host. API calls proxy to the Gateway at
 * :8080 via devServer.
 */

function NavBar() {
  const navigate = useNavigate();
  const logout = async () => {
    try {
      await api.logout();
    } catch {
      // token may already be blacklisted — still clear local state
    }
    localStorage.removeItem('b_token');
    bus.emit(bus.events.AUTH_LOGOUT);
    navigate('/login');
  };
  return (
    <nav className="standalone-nav">
      <strong>B · User Remote（独立模式）</strong>
      <Link to="/login">登录/注册</Link>
      <Link to="/profile">我的</Link>
      <button type="button" className="logout" onClick={logout}>
        退出
      </button>
    </nav>
  );
}

function App() {
  return (
    <BrowserRouter>
      <NavBar />
      <Routes>
        <Route path="/" element={<Navigate to="/login" replace />} />
        {/* Auth.jsx navigates to /home after login — land on own profile here */}
        <Route path="/home" element={<Navigate to="/profile" replace />} />
        <Route path="/login" element={<Auth />} />
        <Route path="/register" element={<Auth />} />
        <Route path="/profile" element={<Profile />} />
        <Route path="/profile/:id" element={<Profile />} />
        <Route path="*" element={<Link to="/login">返回登录</Link>} />
      </Routes>
    </BrowserRouter>
  );
}

createRoot(document.getElementById('root')).render(<App />);
