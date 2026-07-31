import { bus } from './event-bus';

/**
 * @b/shared — coffee-brown UI kit (极简主义, black/white + #8B5A2B accent).
 * All styling via plain CSS + CSS variables. No CSS-in-JS.
 * Contract: { Avatar, Button, Modal, Toast, Skeleton, EmptyState }
 */

export function Avatar({ src, name = '', size = 48 }) {
  const style = { width: size, height: size, fontSize: size * 0.42 };
  const initial = (name || '?').trim().charAt(0).toUpperCase();
  if (src) {
    return <img className="b-avatar" style={style} src={src} alt={name} />;
  }
  return (
    <span className="b-avatar" style={style} aria-label={name}>
      {initial}
    </span>
  );
}

export function Button({ variant = 'primary', disabled, onClick, children, loading, className = '', ...rest }) {
  const cls = ['btn', `btn--${variant}`, loading ? 'is-loading' : '', className].filter(Boolean).join(' ');
  return (
    <button
      type="button"
      className={cls}
      disabled={disabled || loading}
      onClick={onClick}
      {...rest}
    >
      {loading ? '加载中…' : children}
    </button>
  );
}

export function Modal({ open, onClose, title, children, width = 480 }) {
  if (!open) return null;
  return (
    <div
      className="b-modal-overlay"
      onClick={onClose}
      role="dialog"
      aria-modal="true"
      aria-label={title}
    >
      <div
        className="b-modal"
        style={{ width }}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="b-modal__head">
          <h3 className="b-modal__title">{title}</h3>
          <button className="b-modal__close" onClick={onClose} aria-label="关闭">
            ×
          </button>
        </div>
        <div className="b-modal__body">{children}</div>
      </div>
    </div>
  );
}

const TOAST_EVENT = 'toast:show';

/**
 * Toast — bus-driven singleton.
 * ui.Toast.show({type, message}) only emits a bus event;
 * <ToastHost/> (mounted once in Shell App) stacks toasts, auto-dismiss 3s.
 */
export const Toast = {
  show({ type = 'success', message }) {
    bus.emit(TOAST_EVENT, { type, message, id: `${Date.now()}-${Math.random().toString(36).slice(2, 7)}` });
  },
};

export function ToastHost() {
  const [toasts, setToasts] = React.useState([]);
  React.useEffect(() => {
    const off = bus.on(TOAST_EVENT, (toast) => {
      setToasts((prev) => [...prev, toast]);
      setTimeout(() => {
        setToasts((prev) => prev.filter((t) => t.id !== toast.id));
      }, 3000);
    });
    return () => off();
  }, []);
  return (
    <div className="b-toasts" aria-live="polite">
      {toasts.map((t) => (
        <div key={t.id} className={`b-toast b-toast--${t.type}`}>
          <span className="b-toast__icon">{t.type === 'error' ? '✕' : '✓'}</span>
          <span>{t.message}</span>
        </div>
      ))}
    </div>
  );
}

export function Skeleton({ width, height, circle = false, className = '' }) {
  const style = {
    width: circle ? height : width,
    height,
    borderRadius: circle ? '50%' : '8px',
  };
  return <div className={`b-skeleton ${className}`} style={style} />;
}

export function EmptyState({ icon = '📭', title, description, action }) {
  return (
    <div className="b-empty">
      <div className="b-empty__icon">{icon}</div>
      <h3 className="b-empty__title">{title}</h3>
      {description && <p className="b-empty__desc">{description}</p>}
      {action}
    </div>
  );
}

export const ui = { Avatar, Button, Modal, Toast, Skeleton, EmptyState, ToastHost };
