import React from 'react';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { ui } from './shared';
import Layout from './layout/Layout';
import Protected from './components/Protected';
import ErrorBoundary from './components/ErrorBoundary';

// Feed Remote (:3001) — lazy-load each exposed component behind ErrorBoundary.
const Feed = {
  HomeTimeline: React.lazy(() => import('feed/HomeTimeline')),
  PostDetail: React.lazy(() => import('feed/PostDetail')),
  Explore: React.lazy(() => import('feed/Explore')),
};

// User Remote (:3002) — Auth (login/register) + Profile.
const User = {
  Auth: React.lazy(() => import('user/Auth')),
  Profile: React.lazy(() => import('user/Profile')),
};

const PageFallback = () => (
  <div className="page" style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
    <ui.Skeleton height={90} />
    <ui.Skeleton height={120} />
    <ui.Skeleton height={120} />
  </div>
);

function RemotePage({ Component }) {
  return (
    <ErrorBoundary>
      <React.Suspense fallback={<PageFallback />}>
        <Component />
      </React.Suspense>
    </ErrorBoundary>
  );
}

export default function App() {
  return (
    <BrowserRouter>
      <ui.ToastHost />
      <Routes>
        <Route path="/" element={<Navigate to="/home" replace />} />
        <Route path="/login" element={<RemotePage Component={User.Auth} />} />
        <Route path="/register" element={<RemotePage Component={User.Auth} />} />
        <Route
          element={
            <Protected>
              <Layout />
            </Protected>
          }
        >
          <Route path="/home" element={<RemotePage Component={Feed.HomeTimeline} />} />
          <Route path="/explore" element={<RemotePage Component={Feed.Explore} />} />
          <Route path="/post/:id" element={<RemotePage Component={Feed.PostDetail} />} />
          <Route path="/profile/:id" element={<RemotePage Component={User.Profile} />} />
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </BrowserRouter>
  );
}
