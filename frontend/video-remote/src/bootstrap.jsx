import React from 'react';
import { createRoot } from 'react-dom/client';
import App from './App';

// Standalone dev mount (webpack.standalone.js entry). In the Module Federation
// build this chunk is never executed — the Shell host lazy-loads the exposed
// components directly via `video/VideoUpload` etc.
createRoot(document.getElementById('root')).render(<App />);