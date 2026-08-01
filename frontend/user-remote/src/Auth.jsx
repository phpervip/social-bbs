import { useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { api, bus, ui } from '@b/shared';
import './styles.css'; // required — component styles must travel with the lazy MF chunk

/**
 * Auth — /login + /register : dual-tab login/register card.
 * Login: account (username or email) + password.
 * Register: username + display_name (optional) + email + password.
 * On success stores the JWT under the shared 'b_token' key, emits
 * AUTH_LOGIN, and navigates to /home (Shell's protected home timeline).
 */
export default function Auth() {
  const navigate = useNavigate();
  const location = useLocation();
  const [tab, setTab] = useState(location.pathname === '/register' ? 'register' : 'login');
  const [submitting, setSubmitting] = useState(false);

  // login fields
  const [account, setAccount] = useState('');
  const [password, setPassword] = useState('');
  // register fields
  const [username, setUsername] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [email, setEmail] = useState('');
  const [regPassword, setRegPassword] = useState('');

  const isLogin = tab === 'login';
  const canSubmit = isLogin
    ? Boolean(account.trim() && password)
    : Boolean(username.trim() && email.trim() && regPassword);

  // Shared post-auth flow: persist token (same key as the Shell api-client),
  // notify the bus, then move to the home timeline.
  const finish = (data) => {
    localStorage.setItem('b_token', data.token);
    bus.emit(bus.events.AUTH_LOGIN, data.token);
    navigate('/home');
  };

  const submit = async () => {
    if (!canSubmit || submitting) return;
    setSubmitting(true);
    try {
      const data = isLogin
        ? await api.login({ account: account.trim(), password })
        : await api.register({
            username: username.trim(),
            display_name: displayName.trim(),
            email: email.trim(),
            password: regPassword,
          });
      finish(data);
    } catch (e) {
      ui.Toast.show({ type: 'error', message: e.message || (isLogin ? '登录失败，请重试' : '注册失败，请重试') });
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="user-auth">
      <div className="user-auth-card">
        <div className="user-logo">B</div>
        <h1 className="user-auth-title">{isLogin ? '登录 B' : '注册 B'}</h1>
        <p className="user-auth-sub">{isLogin ? '欢迎回来' : '加入 B，开始分享'}</p>

        <div className="user-tabs" role="tablist">
          <button
            type="button"
            role="tab"
            aria-selected={isLogin}
            className={`user-tab${isLogin ? ' active' : ''}`}
            onClick={() => setTab('login')}
          >
            登录
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={!isLogin}
            className={`user-tab${isLogin ? '' : ' active'}`}
            onClick={() => setTab('register')}
          >
            注册
          </button>
        </div>

        <form className="user-auth-form" onSubmit={(e) => { e.preventDefault(); submit(); }}>
          {isLogin ? (
            <>
              <input
                className="user-input"
                placeholder="用户名或邮箱"
                value={account}
                autoFocus
                onChange={(e) => setAccount(e.target.value)}
              />
              <input
                className="user-input"
                type="password"
                placeholder="密码"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </>
          ) : (
            <>
              <input
                className="user-input"
                placeholder="用户名"
                value={username}
                autoFocus
                onChange={(e) => setUsername(e.target.value)}
              />
              <input
                className="user-input"
                placeholder="昵称（可选）"
                value={displayName}
                onChange={(e) => setDisplayName(e.target.value)}
              />
              <input
                className="user-input"
                type="email"
                placeholder="邮箱"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
              />
              <input
                className="user-input"
                type="password"
                placeholder="密码"
                value={regPassword}
                onChange={(e) => setRegPassword(e.target.value)}
              />
            </>
          )}
          <ui.Button
            variant="primary"
            className="user-btn-block"
            disabled={!canSubmit}
            loading={submitting}
            onClick={submit}
          >
            {isLogin ? '登录' : '注册'}
          </ui.Button>
        </form>
      </div>
    </div>
  );
}
