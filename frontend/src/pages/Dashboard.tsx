import { useQuery } from '@tanstack/react-query'
import { getStats, getExits } from '../api/client'
import {
  Globe, ListFilter, Rss, Activity, CheckCircle, XCircle,
  AlertCircle, RotateCcw, ArrowRight,
} from 'lucide-react'
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
  const { data: stats, isLoading: statsLoading } = useQuery({
    queryKey: ['stats'],
    queryFn: getStats,
    refetchInterval: 15_000,
  })
  const { data: exits, isLoading: exitsLoading } = useQuery({
    queryKey: ['exits'],
    queryFn: getExits,
    refetchInterval: 15_000,
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
      ) : !statsLoading && (
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
