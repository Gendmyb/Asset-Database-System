// Layout — 侧边栏 + 顶栏
import { Outlet, Link, useNavigate, useLocation } from 'react-router-dom'
import { useAuthStore } from '../store/authStore'

const navItems = [
  { path: '/dashboard', label: '仪表盘' },
  { path: '/assets', label: '资产管理' },
  { path: '/agents', label: 'Agent' },
  { path: '/admin', label: '管理' },
]

export default function Layout() {
  const { user, logout } = useAuthStore()
  const navigate = useNavigate()
  const { pathname } = useLocation()

  return (
    <div className="flex h-screen">
      {/* 侧边栏 */}
      <aside className="w-60 bg-gray-900 text-white flex flex-col">
        <div className="p-4 text-lg font-bold border-b border-gray-700">
          Asset DB
        </div>
        <nav className="flex-1 p-2">
          {navItems.map((item) => (
            <Link
              key={item.path}
              to={item.path}
              className={`block px-3 py-2 rounded mb-1 text-sm ${
                pathname.startsWith(item.path)
                  ? 'bg-blue-600 text-white'
                  : 'text-gray-300 hover:bg-gray-800'
              }`}
            >
              {item.label}
            </Link>
          ))}
        </nav>
        <div className="p-4 border-t border-gray-700 text-sm">
          <div>{user?.username}</div>
          <div className="text-gray-400 text-xs">{user?.role}</div>
          <button
            onClick={() => { logout(); navigate('/login') }}
            className="mt-2 text-red-400 hover:text-red-300 text-xs"
          >
            退出登录
          </button>
        </div>
      </aside>

      {/* 主内容 */}
      <main className="flex-1 overflow-auto bg-gray-50">
        <div className="p-6">
          <Outlet />
        </div>
      </main>
    </div>
  )
}
