import React from 'react';
import { BrowserRouter, Routes, Route } from 'react-router-dom';
import VideoUpload from './components/VideoUpload';
import VideoPlayer from './components/VideoPlayer';
import VideoList from './components/VideoList';

/**
 * Standalone router for dev mode (npm run dev:standalone) — NOT part of the
 * Module Federation build. The Shell host (:3000) provides its own routes.
 */
export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/upload" element={<VideoUpload />} />
        <Route path="/video/:id" element={<VideoPlayer />} />
        <Route path="/" element={<VideoList />} />
      </Routes>
    </BrowserRouter>
  );
}