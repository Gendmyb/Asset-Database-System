import { Routes, Route, Navigate } from 'react-router-dom'
import Layout from './components/Layout'
import RequireAuth from './components/RequireAuth'
import Login from './pages/Login'
import Assets from './pages/Assets'
import Dashboard from './pages/Dashboard'
import Agents from './pages/Agents'

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route
        element={
          <RequireAuth>
            <Layout />
          </RequireAuth>
        }
      >
        <Route path="/" element={<Navigate to="/assets" replace />} />
        <Route path="/assets" element={<Assets />} />
        <Route path="/assets/:id" element={<Assets />} />
        <Route path="/dashboard" element={<Dashboard />} />
        <Route path="/agents" element={<Agents />} />
        <Route path="/admin" element={<div className="p-8">Admin (super_admin only)</div>} />
      </Route>
    </Routes>
  )
}
