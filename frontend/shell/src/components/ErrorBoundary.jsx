import React from 'react';

/**
 * ErrorBoundary — catches render errors (incl. remote module failures),
 * shows fallback UI with a reload button.
 */
export default class ErrorBoundary extends React.Component {
  constructor(props) {
    super(props);
    this.state = { hasError: false };
  }

  static getDerivedStateFromError() {
    return { hasError: true };
  }

  componentDidCatch(error, info) {
    console.error('[ErrorBoundary]', error, info);
  }

  handleReload = () => {
    window.location.reload();
  };

  render() {
    if (this.state.hasError) {
      return (
        <div className="b-empty">
          <div className="b-empty__icon">⚠️</div>
          <h3 className="b-empty__title">页面加载出错</h3>
          <p className="b-empty__desc">远程模块可能尚未就绪，请重试。</p>
          <button type="button" className="btn btn--primary" onClick={this.handleReload}>
            重新加载
          </button>
        </div>
      );
    }
    return this.props.children;
  }
}
