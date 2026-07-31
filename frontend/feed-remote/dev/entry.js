import { createRoot } from 'react-dom/client';
import { BrowserRouter, Link, Navigate, Route, Routes } from 'react-router-dom';
import HomeTimeline from '../src/HomeTimeline.jsx';
import PostDetail from '../src/PostDetail.jsx';
import Explore from '../src/Explore.jsx';
import '../src/styles.css';

/**
 * Standalone dev shell — NOT part of the Module Federation build.
 * Bundles the exposed pages with a local @b/shared shim (webpack.standalone.js
 * aliases '@b/shared' → dev/shared-shim.js) so the UI can be verified without
 * the Shell host. API calls proxy to the Gateway at :8080 via devServer.
 */
function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<Navigate to="/home" replace />} />
        <Route path="/home" element={<HomeTimeline />} />
        <Route path="/post/:id" element={<PostDetail />} />
        <Route path="/explore" element={<Explore />} />
        <Route path="*" element={<Link to="/home">返回首页</Link>} />
      </Routes>
    </BrowserRouter>
  );
}

createRoot(document.getElementById('root')).render(<App />);
