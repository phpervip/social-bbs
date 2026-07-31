import React from 'react';
import { useNavigate, useLocation } from 'react-router-dom';
import { api, ui } from '../shared';
import Logo from '../layout/Logo';

/**
 * Login — standalone dev login page (no Layout).
 * Fetches seed users via api.devUsers(), click → api.devLogin(id) → /home.
 */
export default function Login() {
  const navigate = useNavigate();
  const location = useLocation();
  const [users, setUsers] = React.useState([]);
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState('');

  React.useEffect(() => {
    let cancelled = false;
    api
      .devUsers()
      .then((data) => {
        if (!cancelled) setUsers(Array.isArray(data) ? data : []);
      })
      .catch((err) => {
        if (!cancelled) setError(err.message || '获取账号列表失败');
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const handleLogin = async (user) => {
    try {
      await api.devLogin(user.id);
      const from = location.state?.from || '/home';
      navigate(from, { replace: true });
    } catch (err) {
      setError(err.message || '登录失败');
    }
  };

  return (
    <div className="login">
      <div className="login__card">
        <Logo size={64} />
        <h1 className="login__title">欢迎来到 B</h1>
        <p className="login__sub">选择演示账号登录</p>

        {loading && (
          <div className="login__users">
            <ui.Skeleton height={56} />
            <ui.Skeleton height={56} />
            <ui.Skeleton height={56} />
            <ui.Skeleton height={56} />
          </div>
        )}

        {!loading && error && (
          <div className="login__error">
            <p>{error}</p>
            <ui.Button
              variant="ghost"
              onClick={() => {
                setError('');
                setLoading(true);
                api
                  .devUsers()
                  .then((data) => setUsers(Array.isArray(data) ? data : []))
                  .catch((err) => setError(err.message || '获取账号列表失败'))
                  .finally(() => setLoading(false));
              }}
            >
              重试
            </ui.Button>
          </div>
        )}

        {!loading && !error && (
          <div className="login__users">
            {users.map((user) => (
              <button
                key={user.id}
                type="button"
                className="login__user"
                onClick={() => handleLogin(user)}
              >
                <ui.Avatar src={user.avatar_url} name={user.display_name} size={44} />
                <span className="login__user-info">
                  <span className="login__user-name">{user.display_name}</span>
                  <span className="login__user-id">@{user.username}</span>
                </span>
                <span className="login__user-arrow">→</span>
              </button>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
