import { useQuery } from '@tanstack/react-query'
import { getStats, getExits, getApiError } from '../api/client'
import {
  Globe, ListFilter, Rss, Activity, CheckCircle, XCircle,
  AlertCircle, RotateCcw, ArrowRight,
} from 'lucide-react'
import { useState } from 'react'
import { Link } from 'react-router-dom'

function StatusDot({ status }: { status: string }) {
  if (status === 'running') return <span className="status-dot-running" />
  if (status === 'unhealthy') return <span className="status-dot-unhealthy" />
  return <span className="status-dot-stopped" />
}

function ExitStatusBadge({ status }: { status: string }) {
  if (status === 'running') return <span className="badge-green">Running</span>
  if (status === 'stopped') return <span className="badge-gray">Stopped</span>
  if (status === 'unhealthy') return <span className="badge-yellow">Unhealthy</span>
  return <span className="badge-gray">{status}</span>
}

export default function Dashboard() {
  const [autoRefresh, setAutoRefresh] = useState(true)
  const { data: stats, isLoading: statsLoading, error: statsError, refetch: refreshStats, isFetching: statsFetching, dataUpdatedAt } = useQuery({
    queryKey: ['stats'],
    queryFn: getStats,
    refetchInterval: autoRefresh ? 15_000 : false,
  })
  const { data: exits, isLoading: exitsLoading, error: exitsError, refetch: refreshExits, isFetching: exitsFetching } = useQuery({
    queryKey: ['exits'],
    queryFn: getExits,
    refetchInterval: autoRefresh ? 15_000 : false,
  })

  const statCards = [
    {
      label: 'Active Rules',
      value: stats?.rule_count ?? '—',
      icon: ListFilter,
      color: 'text-indigo-400',
      bg: 'bg-indigo-950/50',
      border: 'border-indigo-800/30',
      to: '/rules',
    },
    {
      label: 'Subscriptions',
      value: stats?.sub_count ?? '—',
      icon: Rss,
      color: 'text-blue-400',
      bg: 'bg-blue-950/50',
      border: 'border-blue-800/30',
      to: '/subscriptions',
    },
    {
      label: 'Total Exits',
      value: stats?.exit_count ?? '—',
      icon: Globe,
      color: 'text-emerald-400',
      bg: 'bg-emerald-950/50',
      border: 'border-emerald-800/30',
      to: '/exits',
    },
    {
      label: 'Running Exits',
      value: stats?.running_exit_count ?? '—',
      icon: Activity,
      color: 'text-green-400',
      bg: 'bg-green-950/50',
      border: 'border-green-800/30',
      to: '/exits',
    },
  ]

  return (
    <div className="space-y-6">
      {/* Page header */}
      <div>
        <h1 className="page-title">Dashboard</h1>
        <p className="page-subtitle">SNI traffic routing overview</p>
      </div>

      <div className="flex flex-wrap items-center gap-3 text-sm text-gray-400">
        <label className="flex items-center gap-2">
          <input type="checkbox" checked={autoRefresh} onChange={e => setAutoRefresh(e.target.checked)} />
          Auto-refresh every 15 seconds
        </label>
        <button className="btn-secondary" disabled={statsFetching || exitsFetching}
          onClick={() => { void refreshStats(); void refreshExits() }}>
          <RotateCcw size={14} className={statsFetching || exitsFetching ? 'animate-spin' : ''} /> Refresh now
        </button>
        {dataUpdatedAt > 0 && <span className="text-xs">Last received: {new Date(dataUpdatedAt).toLocaleTimeString()}</span>}
      </div>
      {(statsError || exitsError) && (
        <div role="alert" className="card border-red-800 text-red-300 text-sm">
          Could not refresh gateway status: {getApiError(statsError || exitsError)}. Check your connection and try Refresh now.
          {(stats || exits) && <p className="mt-1">Previously received data is shown below.</p>}
        </div>
      )}
      {stats && !statsError && !exitsError && (!stats.exit_count || !stats.rule_count || !stats.last_publish?.id) && (
        <section className="card space-y-4" aria-labelledby="setup-title">
          <h2 id="setup-title" className="font-semibold text-white">Get your gateway ready</h2>
          <p className="text-sm text-gray-400">Follow these steps to route your first domain through a VPN.</p>
          <ol className="grid gap-3 sm:grid-cols-3">
            {[
              { title: '1. Connect a VPN exit', detail: 'Add your VPN credentials, start the exit, and check its public IP.', to: '/exits', done: stats.running_exit_count > 0 },
              { title: '2. Choose your domains', detail: 'Add routing rules or subscribe to an automatically updated domain list.', to: '/rules', done: stats.rule_count > 0 },
              { title: '3. Publish your routes', detail: 'Preview and apply your configuration, then point domain DNS to this gateway.', to: '/publish', done: stats.last_publish?.status === 'success' },
            ].map(step => (
              <li key={step.to} className="rounded-xl border border-gray-700 p-4">
                <Link to={step.to} className="text-indigo-300 font-medium text-sm">{step.title} {step.done && '✓'}</Link>
                <p className="mt-2 text-xs text-gray-400">{step.detail}</p>
              </li>
            ))}
          </ol>
          <Link to="/subscriptions" className="text-sm text-indigo-300">Set up automatic domain-list updates →</Link>
        </section>
      )}

      {/* Stat cards */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        {statCards.map(({ label, value, icon: Icon, color, bg, border, to }) => (
          <Link
            key={label}
            to={to}
            className={`card border ${border} ${bg} hover:border-opacity-60 transition-all group`}
          >
            <div className="flex items-center justify-between mb-3">
              <span className="text-xs font-medium text-gray-500">{label}</span>
              <div className={`w-8 h-8 rounded-lg ${bg} flex items-center justify-center`}>
                <Icon size={15} className={color} />
              </div>
            </div>
            <div className="text-3xl font-bold text-white tabular-nums">{value}</div>
            <div className="flex items-center gap-1 mt-2 text-xs text-gray-600 group-hover:text-gray-400 transition-colors">
              <span>View</span>
              <ArrowRight size={11} />
            </div>
          </Link>
        ))}
      </div>

      {/* Exits table */}
      <div className="card p-0 overflow-hidden">
        <div className="flex items-center justify-between px-5 py-4 border-b border-gray-800/80">
          <h2 className="text-sm font-semibold text-gray-300">Exit Status</h2>
          <Link to="/exits" className="text-xs text-indigo-400 hover:text-indigo-300 transition-colors flex items-center gap-1">
            Manage <ArrowRight size={12} />
          </Link>
        </div>
        {exitsLoading ? (
          <div className="px-5 py-8 text-center text-gray-600 text-sm">Loading...</div>
        ) : exitsError && !exits ? (
          <p className="p-5 text-sm text-gray-400">Exit status is unavailable. Use Refresh now to retry.</p>
        ) : !exits || exits.length === 0 ? (
          <div className="px-5 py-10 text-center">
            <Globe size={28} className="mx-auto text-gray-700 mb-2" />
            <p className="text-gray-500 text-sm">No exits configured</p>
            <Link to="/exits" className="btn-primary mt-3 text-xs inline-flex">
              Add your first exit
            </Link>
          </div>
        ) : (
          <div>
            {exits.map((exit, i) => (
              <div
                key={exit.id}
                className={`flex items-center justify-between px-5 py-3 ${
                  i < exits.length - 1 ? 'border-b border-gray-800/50' : ''
                } hover:bg-gray-800/30 transition-colors`}
              >
                <div className="flex items-center gap-3">
                  <StatusDot status={exit.status} />
                  <div>
                    <div className="font-medium text-sm text-white">{exit.name}</div>
                    <div className="text-xs text-gray-500 mt-0.5">
                      {exit.provider} · {exit.protocol} · {exit.country}
                      {exit.city ? ` (${exit.city})` : ''}
                    </div>
                  </div>
                </div>
                <div className="flex items-center gap-3">
                  {exit.public_ip && (
                    <span className="text-xs text-gray-500 font-mono hidden sm:block">
                      {exit.public_ip}
                    </span>
                  )}
                  <ExitStatusBadge status={exit.status} />
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Last publish */}
      {stats?.last_publish?.id ? (
        <div className="card">
          <div className="flex items-center justify-between mb-3">
            <h2 className="text-sm font-semibold text-gray-300">Last Publish</h2>
            <Link to="/publish" className="text-xs text-indigo-400 hover:text-indigo-300 transition-colors flex items-center gap-1">
              Publish <ArrowRight size={12} />
            </Link>
          </div>
          <div className="flex items-center gap-3">
            {stats.last_publish.status === 'success' ? (
              <CheckCircle size={16} className="text-green-400 flex-shrink-0" />
            ) : stats.last_publish.status === 'failed' ? (
              <XCircle size={16} className="text-red-400 flex-shrink-0" />
            ) : (
              <RotateCcw size={16} className="text-yellow-400 flex-shrink-0" />
            )}
            <div className="flex-1 min-w-0">
              <span className="text-sm font-mono text-white">{stats.last_publish.version}</span>
              <span className="text-xs text-gray-500 ml-2">{stats.last_publish.message}</span>
            </div>
            <span className="text-xs text-gray-600 flex-shrink-0">
              {new Date(stats.last_publish.created_at).toLocaleString()}
            </span>
          </div>
        </div>
      ) : !statsLoading && !statsError && (
        <div className="card border-dashed text-center py-6">
          <AlertCircle size={20} className="mx-auto text-gray-700 mb-2" />
          <p className="text-gray-500 text-sm">No publish history</p>
          <Link to="/publish" className="btn-primary mt-3 text-xs inline-flex">
            Publish config
          </Link>
        </div>
      )}
    </div>
  )
}
