import { useQuery } from '@tanstack/react-query'
import { getStats, getExits } from '../api/client'
import { Globe, ListFilter, Rss, Activity, CheckCircle, XCircle, AlertCircle } from 'lucide-react'

function StatusDot({ status }: { status: string }) {
  if (status === 'running') return <span className="w-2 h-2 rounded-full bg-green-400 inline-block" />
  if (status === 'stopped') return <span className="w-2 h-2 rounded-full bg-gray-500 inline-block" />
  return <span className="w-2 h-2 rounded-full bg-yellow-400 inline-block" />
}

export default function Dashboard() {
  const { data: stats } = useQuery({ queryKey: ['stats'], queryFn: getStats, refetchInterval: 15_000 })
  const { data: exits } = useQuery({ queryKey: ['exits'], queryFn: getExits, refetchInterval: 15_000 })

  const statCards = [
    {
      label: '啟用規則',
      value: stats?.rule_count ?? '—',
      icon: ListFilter,
      color: 'text-indigo-400',
    },
    {
      label: '啟用訂閱',
      value: stats?.sub_count ?? '—',
      icon: Rss,
      color: 'text-blue-400',
    },
    {
      label: '出口總數',
      value: stats?.exit_count ?? '—',
      icon: Globe,
      color: 'text-emerald-400',
    },
    {
      label: '執行中出口',
      value: stats?.running_exit_count ?? '—',
      icon: Activity,
      color: 'text-green-400',
    },
  ]

  return (
    <div className="space-y-6 max-w-5xl">
      <div>
        <h1 className="text-2xl font-bold text-white">Dashboard</h1>
        <p className="text-sm text-gray-400 mt-1">SNI 分流閘道總覽</p>
      </div>

      {/* Stat cards */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        {statCards.map(({ label, value, icon: Icon, color }) => (
          <div key={label} className="card">
            <div className="flex items-center justify-between mb-3">
              <span className="text-xs text-gray-400">{label}</span>
              <Icon size={16} className={color} />
            </div>
            <div className="text-3xl font-bold text-white">{value}</div>
          </div>
        ))}
      </div>

      {/* Exits overview */}
      <div className="card">
        <h2 className="text-sm font-semibold text-gray-300 mb-4">出口狀態</h2>
        {!exits || exits.length === 0 ? (
          <p className="text-sm text-gray-500">尚未設定任何出口。請前往「出口管理」新增。</p>
        ) : (
          <div className="space-y-2">
            {exits.map(exit => (
              <div key={exit.id} className="flex items-center justify-between py-2 border-b border-gray-800 last:border-0">
                <div className="flex items-center gap-3">
                  <StatusDot status={exit.status} />
                  <div>
                    <div className="font-medium text-sm text-white">{exit.name}</div>
                    <div className="text-xs text-gray-500">{exit.provider} · {exit.country} {exit.city}</div>
                  </div>
                </div>
                <div className="flex items-center gap-3">
                  {exit.public_ip && (
                    <span className="text-xs text-gray-400 font-mono">{exit.public_ip}</span>
                  )}
                  <ExitStatusBadge status={exit.status} />
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Last publish */}
      {stats?.last_publish?.id && (
        <div className="card">
          <h2 className="text-sm font-semibold text-gray-300 mb-3">最近發布</h2>
          <div className="flex items-center gap-3">
            {stats.last_publish.status === 'success'
              ? <CheckCircle size={16} className="text-green-400" />
              : stats.last_publish.status === 'failed'
              ? <XCircle size={16} className="text-red-400" />
              : <AlertCircle size={16} className="text-yellow-400" />
            }
            <div>
              <span className="text-sm text-white">{stats.last_publish.version}</span>
              <span className="text-xs text-gray-500 ml-2">{stats.last_publish.message}</span>
            </div>
            <span className="ml-auto text-xs text-gray-500">
              {new Date(stats.last_publish.created_at).toLocaleString('zh-TW')}
            </span>
          </div>
        </div>
      )}
    </div>
  )
}

function ExitStatusBadge({ status }: { status: string }) {
  if (status === 'running') return <span className="badge-green">執行中</span>
  if (status === 'stopped') return <span className="badge-gray">已停止</span>
  if (status === 'unhealthy') return <span className="badge-red">不健康</span>
  return <span className="badge-yellow">{status}</span>
}
