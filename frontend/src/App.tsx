import { BrowserRouter, Routes, Route, NavLink } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  LayoutDashboard, ListFilter, Rss, Globe, Upload, Menu, X, LogOut, User,
} from 'lucide-react'
import { useState, useEffect } from 'react'
import Dashboard from './pages/Dashboard'
import Rules from './pages/Rules'
import Subscriptions from './pages/Subscriptions'
import Exits from './pages/Exits'
import Publish from './pages/Publish'
import Login from './pages/Login'
import { ToastProvider } from './components/Toast'
import { clearAuth, loadAuth, saveAuth, type AuthState } from './auth'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { staleTime: 10_000, retry: 1 },
  },
})

const navItems = [
  { to: '/', label: 'Dashboard', icon: LayoutDashboard, end: true },
  { to: '/exits', label: 'Exits', icon: Globe },
  { to: '/rules', label: 'Rules', icon: ListFilter },
  { to: '/subscriptions', label: 'Subscriptions', icon: Rss },
  { to: '/publish', label: 'Publish', icon: Upload },
]

export default function App() {
  const [auth, setAuth] = useState<AuthState | null>(loadAuth)
  const [mobileOpen, setMobileOpen] = useState(false)

  // Listen for 401 events from the API client
  useEffect(() => {
    const handler = () => {
      setAuth(null)
      setMobileOpen(false)
      queryClient.clear()
    }
    window.addEventListener('sni:logout', handler)
    return () => window.removeEventListener('sni:logout', handler)
  }, [])

  const handleLogin = (token: string, username: string, expiresAt?: number) => {
    setAuth(saveAuth(token, username, expiresAt))
    queryClient.clear()
  }

  const handleLogout = () => {
    clearAuth()
    setAuth(null)
    setMobileOpen(false)
    queryClient.clear()
  }

  if (!auth) {
    return (
      <ToastProvider>
        <Login onLogin={handleLogin} />
      </ToastProvider>
    )
  }

  return (
    <QueryClientProvider client={queryClient}>
      <ToastProvider>
        <BrowserRouter>
          <div className="flex h-screen overflow-hidden bg-gray-950">
            {/* Sidebar */}
            <aside
              className={`
                fixed inset-y-0 left-0 z-40 w-60 bg-gray-900 border-r border-gray-800/80 flex flex-col
                transform transition-transform duration-200
                ${mobileOpen ? 'translate-x-0' : '-translate-x-full'}
                md:relative md:translate-x-0
              `}
            >
              {/* Logo */}
              <div className="flex items-center gap-3 px-5 py-5 border-b border-gray-800/80">
                <div className="w-8 h-8 bg-indigo-600 rounded-lg flex items-center justify-center text-white font-bold text-sm shadow-lg shadow-indigo-900/40 flex-shrink-0">
                  S
                </div>
                <div className="min-w-0">
                  <div className="font-semibold text-white text-sm leading-tight">SNI Gateway</div>
                  <div className="text-xs text-gray-500 truncate">Traffic Routing</div>
                </div>
              </div>

              {/* Nav */}
              <nav className="flex-1 px-3 py-4 space-y-0.5 overflow-y-auto">
                {navItems.map(({ to, label, icon: Icon, end }) => (
                  <NavLink
                    key={to}
                    to={to}
                    end={end}
                    onClick={() => setMobileOpen(false)}
                    className={({ isActive }) =>
                      `flex items-center gap-3 px-3 py-2.5 rounded-xl text-sm font-medium transition-all ${
                        isActive
                          ? 'bg-indigo-600/20 text-indigo-300 border border-indigo-600/30'
                          : 'text-gray-400 hover:text-white hover:bg-gray-800/60 border border-transparent'
                      }`
                    }
                  >
                    <Icon size={16} className="flex-shrink-0" />
                    {label}
                  </NavLink>
                ))}
              </nav>

              {/* User / Logout */}
              <div className="px-3 py-3 border-t border-gray-800/80">
                <div className="flex items-center gap-2 px-3 py-2 rounded-xl bg-gray-800/40">
                  <div className="w-7 h-7 rounded-full bg-indigo-600/30 flex items-center justify-center flex-shrink-0">
                    <User size={13} className="text-indigo-400" />
                  </div>
                  <span className="text-xs text-gray-300 flex-1 truncate font-medium">
                    {auth.username}
                  </span>
                  <button
                    onClick={handleLogout}
                    className="text-gray-500 hover:text-red-400 transition-colors p-1"
                    title="Sign out"
                  >
                    <LogOut size={14} />
                  </button>
                </div>
                <p className="text-xs text-gray-700 text-center mt-2">v1.0.0</p>
              </div>
            </aside>

            {/* Mobile overlay */}
            {mobileOpen && (
              <div
                className="fixed inset-0 z-30 bg-black/60 backdrop-blur-sm md:hidden"
                onClick={() => setMobileOpen(false)}
              />
            )}

            {/* Main content */}
            <div className="flex-1 flex flex-col min-w-0 overflow-hidden">
              {/* Mobile header */}
              <header className="md:hidden flex items-center justify-between px-4 py-3 border-b border-gray-800 bg-gray-900 flex-shrink-0">
                <button
                  onClick={() => setMobileOpen(true)}
                  className="text-gray-400 hover:text-white transition-colors p-1"
                >
                  <Menu size={20} />
                </button>
                <span className="font-semibold text-white">SNI Gateway</span>
                <button
                  onClick={handleLogout}
                  className="text-gray-500 hover:text-red-400 transition-colors p-1"
                >
                  <LogOut size={16} />
                </button>
              </header>

              {/* Page content */}
              <main className="flex-1 overflow-y-auto">
                <div className="p-6 max-w-6xl">
                  <Routes>
                    <Route path="/" element={<Dashboard />} />
                    <Route path="/exits" element={<Exits />} />
                    <Route path="/rules" element={<Rules />} />
                    <Route path="/subscriptions" element={<Subscriptions />} />
                    <Route path="/publish" element={<Publish />} />
                  </Routes>
                </div>
              </main>
            </div>
          </div>
        </BrowserRouter>
      </ToastProvider>
    </QueryClientProvider>
  )
}
