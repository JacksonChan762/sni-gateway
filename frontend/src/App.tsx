import { BrowserRouter, Routes, Route, NavLink } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  LayoutDashboard, ListFilter, Rss, Globe, Upload, Menu, X
} from 'lucide-react'
import { useState } from 'react'
import Dashboard from './pages/Dashboard'
import Rules from './pages/Rules'
import Subscriptions from './pages/Subscriptions'
import Exits from './pages/Exits'
import Publish from './pages/Publish'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { staleTime: 10_000, retry: 1 },
  },
})

const navItems = [
  { to: '/', label: 'Dashboard', icon: LayoutDashboard, end: true },
  { to: '/exits', label: '出口管理', icon: Globe },
  { to: '/rules', label: '規則管理', icon: ListFilter },
  { to: '/subscriptions', label: '訂閱管理', icon: Rss },
  { to: '/publish', label: '發布管理', icon: Upload },
]

export default function App() {
  const [mobileOpen, setMobileOpen] = useState(false)

  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <div className="flex h-screen overflow-hidden">
          {/* Sidebar */}
          <aside className={`
            fixed inset-y-0 left-0 z-40 w-56 bg-gray-900 border-r border-gray-800 flex flex-col
            transform transition-transform duration-200
            ${mobileOpen ? 'translate-x-0' : '-translate-x-full'}
            md:relative md:translate-x-0
          `}>
            <div className="flex items-center gap-3 px-5 py-4 border-b border-gray-800">
              <div className="w-8 h-8 bg-indigo-600 rounded-lg flex items-center justify-center text-white font-bold text-sm">S</div>
              <span className="font-semibold text-white">SNI Gateway</span>
            </div>
            <nav className="flex-1 px-3 py-4 space-y-1">
              {navItems.map(({ to, label, icon: Icon, end }) => (
                <NavLink
                  key={to}
                  to={to}
                  end={end}
                  onClick={() => setMobileOpen(false)}
                  className={({ isActive }) =>
                    `flex items-center gap-3 px-3 py-2 rounded-lg text-sm font-medium transition-colors ${
                      isActive
                        ? 'bg-indigo-600 text-white'
                        : 'text-gray-400 hover:text-white hover:bg-gray-800'
                    }`
                  }
                >
                  <Icon size={16} />
                  {label}
                </NavLink>
              ))}
            </nav>
            <div className="px-5 py-3 border-t border-gray-800 text-xs text-gray-600">
              v1.0.0 · SNI Gateway
            </div>
          </aside>

          {/* Mobile overlay */}
          {mobileOpen && (
            <div
              className="fixed inset-0 z-30 bg-black/50 md:hidden"
              onClick={() => setMobileOpen(false)}
            />
          )}

          {/* Main */}
          <div className="flex-1 flex flex-col min-w-0 overflow-hidden">
            {/* Top bar (mobile) */}
            <header className="md:hidden flex items-center gap-3 px-4 py-3 border-b border-gray-800 bg-gray-900">
              <button onClick={() => setMobileOpen(true)} className="text-gray-400 hover:text-white">
                <Menu size={20} />
              </button>
              <span className="font-semibold text-white">SNI Gateway</span>
            </header>

            {/* Content */}
            <main className="flex-1 overflow-y-auto p-6">
              <Routes>
                <Route path="/" element={<Dashboard />} />
                <Route path="/exits" element={<Exits />} />
                <Route path="/rules" element={<Rules />} />
                <Route path="/subscriptions" element={<Subscriptions />} />
                <Route path="/publish" element={<Publish />} />
              </Routes>
            </main>
          </div>
        </div>
      </BrowserRouter>
    </QueryClientProvider>
  )
}
