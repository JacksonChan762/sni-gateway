import { BrowserRouter, Routes, Route, NavLink, useLocation } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  LayoutDashboard, ListFilter, Rss, Globe, Upload, Menu, X, LogOut, User,
} from 'lucide-react'
import { useState, useEffect, useRef } from 'react'
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

function Sidebar({ username, onLogout, onNavigate, onClose }: {
  username: string
  onLogout: () => void
  onNavigate?: () => void
  onClose?: () => void
}) {
  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex items-center gap-3 border-b border-gray-800/80 px-5 py-5">
        <div aria-hidden="true" className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-indigo-600 text-sm font-bold text-white">
          S
        </div>
        <span className="min-w-0 flex-1 truncate text-sm font-semibold text-white">SNI Gateway</span>
        {onClose && (
          <button type="button" onClick={onClose} aria-label="Close navigation"
            className="btn-ghost -mr-2 min-h-10 min-w-10 justify-center p-0">
            <X size={19} aria-hidden="true" />
          </button>
        )}
      </div>
      <nav aria-label="Main navigation" className="flex-1 space-y-0.5 overflow-y-auto px-3 py-4">
        {navItems.map(({ to, label, icon: Icon, end }) => (
          <NavLink key={to} to={to} end={end} onClick={onNavigate}
            className={({ isActive }) =>
              `flex min-h-11 items-center gap-3 rounded-xl border px-3 py-2.5 text-sm font-medium transition-colors ${
                isActive
                  ? 'border-indigo-600/30 bg-indigo-600/20 text-indigo-300'
                  : 'border-transparent text-gray-400 hover:bg-gray-800/60 hover:text-white'
              }`
            }>
            <Icon size={16} className="shrink-0" aria-hidden="true" />
            {label}
          </NavLink>
        ))}
      </nav>
      <div className="border-t border-gray-800/80 px-3 py-3">
        <div className="flex items-center gap-2 rounded-xl bg-gray-800/40 px-3 py-1.5">
          <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-indigo-600/30">
            <User size={13} className="text-indigo-400" aria-hidden="true" />
          </div>
          <span className="flex-1 truncate text-xs font-medium text-gray-300">{username}</span>
          <button type="button" onClick={onLogout} className="btn-ghost min-h-10 min-w-10 justify-center p-0 hover:text-red-400"
            title="Sign out" aria-label="Sign out">
            <LogOut size={15} aria-hidden="true" />
          </button>
        </div>
      </div>
    </div>
  )
}

function Workspace({ username, onLogout }: { username: string; onLogout: () => void }) {
  const { pathname } = useLocation()
  const previousPath = useRef(pathname)
  const drawer = useRef<HTMLDialogElement>(null)
  const main = useRef<HTMLElement>(null)
  const [mobileOpen, setMobileOpen] = useState(false)

  useEffect(() => {
    const desktop = window.matchMedia('(min-width: 768px)')
    const closeOnDesktop = () => { if (desktop.matches) drawer.current?.close() }
    desktop.addEventListener('change', closeOnDesktop)
    return () => desktop.removeEventListener('change', closeOnDesktop)
  }, [])

  useEffect(() => {
    if (previousPath.current === pathname) return
    previousPath.current = pathname
    drawer.current?.close()
    main.current?.scrollTo({ top: 0 })
    main.current?.focus({ preventScroll: true })
  }, [pathname])

  const closeNavigation = () => drawer.current?.close()

  return (
    <div className="flex h-screen overflow-hidden bg-gray-950 supports-[height:100dvh]:h-[100dvh]">
      <a href="#main-content" className="sr-only focus:not-sr-only focus:fixed focus:left-4 focus:top-4 focus:z-50 focus:rounded-xl focus:bg-indigo-600 focus:px-4 focus:py-3 focus:text-white"
        onClick={event => { event.preventDefault(); main.current?.focus() }}>
        Skip to content
      </a>
      <aside className="hidden w-60 shrink-0 border-r border-gray-800/80 bg-gray-900 md:block">
        <Sidebar username={username} onLogout={onLogout} />
      </aside>
      <dialog ref={drawer} id="mobile-navigation" aria-label="Navigation"
        className="fixed inset-y-0 left-0 right-auto m-0 h-full max-h-none w-64 max-w-[85vw] overflow-hidden border-r border-gray-800 bg-gray-900 p-0 text-gray-100 backdrop:bg-black/60 md:hidden"
        onClose={() => setMobileOpen(false)} onClick={event => { if (event.target === event.currentTarget) closeNavigation() }}>
        <Sidebar username={username} onLogout={onLogout} onNavigate={closeNavigation} onClose={closeNavigation} />
      </dialog>
      <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
        <header className="flex shrink-0 items-center justify-between border-b border-gray-800 bg-gray-900 px-3 py-1.5 md:hidden">
          <button type="button" aria-label="Open navigation" aria-expanded={mobileOpen} aria-controls="mobile-navigation"
            className="btn-ghost min-h-11 min-w-11 justify-center p-0"
            onClick={event => {
              event.currentTarget.focus()
              drawer.current?.showModal()
              setMobileOpen(true)
            }}>
            <Menu size={20} aria-hidden="true" />
          </button>
          <span className="font-semibold text-white">SNI Gateway</span>
          <button type="button" aria-label="Sign out" onClick={onLogout} className="btn-ghost min-h-11 min-w-11 justify-center p-0 hover:text-red-400">
            <LogOut size={17} aria-hidden="true" />
          </button>
        </header>

        <main id="main-content" ref={main} tabIndex={-1} className="flex-1 overflow-y-auto">
          <div className="max-w-6xl p-4 sm:p-6">
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
  )
}

export default function App() {
  const [auth, setAuth] = useState<AuthState | null>(loadAuth)

  // Listen for 401 events from the API client
  useEffect(() => {
    const handler = () => {
      setAuth(null)
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
          <Workspace username={auth.username} onLogout={handleLogout} />
        </BrowserRouter>
      </ToastProvider>
    </QueryClientProvider>
  )
}
