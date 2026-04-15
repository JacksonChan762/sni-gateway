import { useState, type FormEvent } from 'react'
import { AlertCircle, Eye, EyeOff, Shield } from 'lucide-react'
import { getApiError, loginUser } from '../api/client'

interface LoginProps {
  onLogin: (token: string, username: string, expiresAt?: number) => void
}

export default function Login({ onLogin }: LoginProps) {
  const [form, setForm] = useState({ username: '', password: '' })
  const [showPassword, setShowPassword] = useState(false)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    if (loading) return

    const username = form.username.trim()
    const password = form.password
    if (!username || !password) {
      setError('Username and password are required')
      return
    }

    setError('')
    setLoading(true)
    try {
      const res = await loginUser(username, password)
      onLogin(res.token, res.username, res.expires_at)
    } catch (err) {
      setError(getApiError(err) || 'Login failed. Please try again.')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen bg-gray-950 flex items-center justify-center p-4">
      {/* Background decoration */}
      <div className="absolute inset-0 overflow-hidden pointer-events-none">
        <div className="absolute -top-40 -right-40 w-80 h-80 bg-indigo-900/20 rounded-full blur-3xl" />
        <div className="absolute -bottom-40 -left-40 w-80 h-80 bg-indigo-900/20 rounded-full blur-3xl" />
      </div>

      <div className="w-full max-w-sm relative">
        {/* Logo */}
        <div className="text-center mb-8">
          <div className="inline-flex items-center justify-center w-16 h-16 bg-indigo-600 rounded-2xl mb-4 shadow-lg shadow-indigo-900/40">
            <Shield size={30} className="text-white" />
          </div>
          <h1 className="text-3xl font-bold text-white tracking-tight">SNI Gateway</h1>
          <p className="text-gray-400 text-sm mt-2">Sign in to manage your gateway</p>
        </div>

        {/* Form card */}
        <div className="bg-gray-900 border border-gray-800 rounded-2xl p-6 shadow-2xl">
          <form onSubmit={handleSubmit} className="space-y-4">
            <div>
              <label className="block text-xs font-medium text-gray-400 mb-1.5">Username</label>
              <input
                className="input"
                type="text"
                autoComplete="username"
                placeholder="admin"
                value={form.username}
                onChange={e => {
                  setForm(f => ({ ...f, username: e.target.value }))
                  setError('')
                }}
                autoFocus
                required
                disabled={loading}
              />
            </div>

            <div>
              <label className="block text-xs font-medium text-gray-400 mb-1.5">Password</label>
              <div className="relative">
                <input
                  className="input pr-10"
                  type={showPassword ? 'text' : 'password'}
                  autoComplete="current-password"
                  placeholder="••••••••"
                  value={form.password}
                  onChange={e => {
                    setForm(f => ({ ...f, password: e.target.value }))
                    setError('')
                  }}
                  required
                  disabled={loading}
                />
                <button
                  type="button"
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-500 hover:text-gray-300 transition-colors"
                  onClick={() => setShowPassword(v => !v)}
                  disabled={loading}
                  tabIndex={-1}
                  title={showPassword ? 'Hide password' : 'Show password'}
                >
                  {showPassword ? <EyeOff size={16} /> : <Eye size={16} />}
                </button>
              </div>
            </div>

            {error && (
              <div className="flex items-center gap-2 text-red-400 text-sm bg-red-900/20 border border-red-800/50 px-3 py-2.5 rounded-lg">
                <AlertCircle size={16} className="flex-shrink-0" />
                {error}
              </div>
            )}

            <button
              type="submit"
              className="btn-primary w-full justify-center py-2.5 mt-2"
              disabled={loading || !form.username.trim() || !form.password}
            >
              {loading ? (
                <span className="flex items-center gap-2">
                  <svg className="animate-spin h-4 w-4" viewBox="0 0 24 24" fill="none">
                    <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                    <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                  </svg>
                  Signing in...
                </span>
              ) : (
                'Sign In'
              )}
            </button>
          </form>
        </div>

        <p className="text-center text-xs text-gray-600 mt-6">
          SNI Gateway · Web-managed SNI/Host routing
        </p>
      </div>
    </div>
  )
}
