import React from 'react';
import { Navigate, useLocation } from 'react-router-dom';
import { getToken } from '../shared/api-client';

/**
 * Protected — token check wrapper (localStorage 'b_token'; NO React Context).
 * No token → redirect to /login (preserving intended destination).
 */
export default function Protected({ children }) {
  const location = useLocation();
  const token = getToken();
  if (!token) {
    return <Navigate to="/login" replace state={{ from: location.pathname }} />;
  }
  return children;
}
