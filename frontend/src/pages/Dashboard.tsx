import { useQuery } from '@tanstack/react-query'
import { getStats, getExits, getApiError, type Stats } from '../api/client'
import {
  Globe, ListFilter, Rss, Activity, CheckCircle, XCircle,
  AlertCircle, RotateCcw, ArrowRight,
} from 'lucide-react'
import { useState } from 'react'
import { Link } from 'react-router-dom'
import PublishStatusNotice from '../components/PublishStatusNotice'

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

function TLSCapacity({ active, limit, rejected }: NonNullable<Stats['tls']>) {
  const remaining = Math.max(0, limit - active)
  const inUse = active.toLocaleString()
  const capacity = limit.toLocaleString()

  return (
    <section className="card space-y-4" aria-labelledby="tls-capacity-title">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 id="tls-capacity-title" className="text-sm font-semibold text-gray-300">TLS connection capacity</h2>
        {remaining === 0 && <span className="badge-yellow">At capacity</span>}
      </div>
      <dl className="grid grid-cols-2 gap-x-4 gap-y-3 sm:grid-cols-3">
        <div>
          <dt className="text-xs text-gray-400">Connections in use</dt>
          <dd className="text-xl font-semibold text-white tabular-nums">{inUse} <span className="text-sm text-gray-400">/ {capacity}</span></dd>
        </div>
        <div>
          <dt className="text-xs text-gray-400">Slots available</dt>
          <dd className="text-xl font-semibold text-white tabular-nums">{remaining.toLocaleString()}</dd>
        </div>
        <div className="col-span-2 sm:col-span-1">
          <dt className="text-xs text-gray-400">Rejected at capacity since restart</dt>
          <dd className="text-xl font-semibold text-white tabular-nums">{rejected.toLocaleString()}</dd>
        </div>
      </dl>
      <div
        role="meter" aria-labelledby="tls-capacity-title" aria-valuemin={0} aria-valuemax={limit}
        aria-valuenow={Math.min(active, limit)} aria-valuetext={`${inUse} of ${capacity} connection slots used`}
        className="h-2 overflow-hidden rounded-full bg-gray-800"
      >
        <div className={`h-full ${remaining === 0 ? 'bg-amber-400' : 'bg-indigo-400'}`}
          style={{ width: `${Math.min(100, 100 * active / limit)}%` }} />
      </div>
      {remaining === 0 && <p className="text-xs text-amber-300">New connections are rejected while all slots are in use.</p>}
      <details className="border-t border-gray-800 pt-3 text-xs text-gray-400">
        <summary className="cursor-pointer py-1 font-medium text-indigo-300">How to change the limit</summary>
        <ol className="ml-4 mt-2 list-decimal space-y-1.5">
          <li>In the deployment directory, set <code className="text-gray-200">TLS_MAX_CONNECTIONS</code> in <code className="text-gray-200">.env</code> to the desired limit.</li>
          <li>For Docker Compose, run <code className="select-all break-words text-gray-200">docker compose up -d backend</code> to apply the change.</li>
        </ol>
        <p className="mt-2">Recreating the backend disconnects active TLS connections. Check the new limit here once it starts.</p>
      </details>
    </section>
  )
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
  const tls = stats?.tls
  const unhealthyExits = exits?.filter(exit => exit.status === 'unhealthy').length ?? 0

  const statCards = [
    {
      label: 'Enabled Rules',
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
      label: 'Enabled Exits',
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
      to: '/exits?status=running',
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
      <PublishStatusNotice />
      {unhealthyExits > 0 && (
        <Link to="/exits?status=unhealthy" className="card flex items-center gap-2.5 border-amber-800/60 bg-amber-950/40 py-3 text-sm font-medium text-amber-200 hover:bg-amber-950/70">
          <AlertCircle size={16} aria-hidden="true" className="shrink-0" />
          <span className="flex-1">Review {unhealthyExits} unhealthy {unhealthyExits === 1 ? 'exit' : 'exits'}</span>
          <ArrowRight size={14} aria-hidden="true" className="shrink-0" />
        </Link>
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

      {tls && tls.limit > 0 && <TLSCapacity {...tls} />}

      {/* Stat cards */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        {statCards.map(({ label, value, icon: Icon, color, bg, border, to }) => (
          <Link
            key={label}
            to={to}
            className={`card border ${border} ${bg} hover:border-opacity-60 transition-all group`}
          >
            <div className="mb-3 flex items-center justify-between gap-2">
              <span className="min-w-0 text-xs font-medium text-gray-400 [overflow-wrap:anywhere]">{label}</span>
              <div className={`hidden h-8 w-8 shrink-0 items-center justify-center rounded-lg min-[380px]:flex ${bg}`}>
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
          <h2 className="text-sm font-semibold text-gray-300">Last recorded exit status</h2>
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
                className={`flex items-start justify-between gap-3 px-5 py-3 ${
                  i < exits.length - 1 ? 'border-b border-gray-800/50' : ''
                } hover:bg-gray-800/30 transition-colors`}
              >
                <div className="flex min-w-0 items-start gap-3">
                  <div className="mt-1.5 shrink-0">
                    <StatusDot status={exit.status} />
                  </div>
                  <div className="min-w-0 [overflow-wrap:anywhere]">
                    <div className="font-medium text-sm text-white">{exit.name}</div>
                    <div className="text-xs text-gray-500 mt-0.5">
                      {[exit.provider, exit.protocol, exit.country, exit.city].filter(Boolean).join(' · ')}
                    </div>
                    {exit.public_ip && <div className="mt-1 font-mono text-xs text-gray-400">Public IP {exit.public_ip}</div>}
                  </div>
                </div>
                <div className="shrink-0">
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
