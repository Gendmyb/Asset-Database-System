import { lazy, Suspense } from 'react'
import { Routes, Route, Navigate, Outlet } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Toaster } from 'sonner'
import Layout from './components/Layout'
import RequireAuth from './components/RequireAuth'
import Login from './pages/Login'
import Assets from './pages/Assets'
import Dashboard from './pages/Dashboard'
import NotFound from './pages/NotFound'

const AssignmentsPage = lazy(() => import('./pages/AssignmentsPage'))
const MaintenancePage = lazy(() => import('./pages/MaintenancePage'))
const StocktakesPage = lazy(() => import('./pages/StocktakesPage'))
const ReportsPage = lazy(() => import('./pages/ReportsPage'))
const UsersPage = lazy(() => import('./pages/admin/Users'))
const SettingsPage = lazy(() => import('./pages/admin/Settings'))

function PageLoader() {
  return (
    <div style={{ display: 'flex', justifyContent: 'center', padding: 80 }}>
      <div
        style={{
          width: 24,
          height: 24,
          border: '2px solid var(--border-default)',
          borderTopColor: 'var(--brand)',
          borderRadius: '50%',
          animation: 'spin 0.6s linear infinite',
        }}
      />
      <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
    </div>
  )
}

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30000,
      retry: 1,
    },
  },
})

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <Toaster
        position="top-right"
        toastOptions={{
          style: {
            background: 'var(--bg-surface)',
            color: 'var(--text-primary)',
            border: '1px solid var(--border-default)',
            borderRadius: '8px',
          },
        }}
      />
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
          <Route
            path="/assignments"
            element={
              <Suspense fallback={<PageLoader />}>
                <AssignmentsPage />
              </Suspense>
            }
          />
          <Route
            path="/maintenance"
            element={
              <Suspense fallback={<PageLoader />}>
                <MaintenancePage />
              </Suspense>
            }
          />
          <Route
            path="/stocktakes"
            element={
              <Suspense fallback={<PageLoader />}>
                <StocktakesPage />
              </Suspense>
            }
          />
          <Route
            path="/reports"
            element={
              <Suspense fallback={<PageLoader />}>
                <ReportsPage />
              </Suspense>
            }
          />
          <Route path="/admin" element={<Outlet />}>
            <Route
              index
              element={<Navigate to="/admin/users" replace />}
            />
            <Route
              path="users"
              element={
                <Suspense fallback={<PageLoader />}>
                  <UsersPage />
                </Suspense>
              }
            />
            <Route
              path="settings"
              element={
                <Suspense fallback={<PageLoader />}>
                  <SettingsPage />
                </Suspense>
              }
            />
          </Route>
          <Route path="*" element={<NotFound />} />
        </Route>
      </Routes>
    </QueryClientProvider>
  )
}
