import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  getExits, createExit, updateExit, deleteExit,
  startExit, stopExit, restartExit, testExit, getExitLogs
} from '../api/client'
import { type Exit } from '../api/client'
import {
  Plus, Play, Square, RotateCcw, Trash2, Terminal,
  RefreshCw, Globe, X, ChevronDown
} from 'lucide-react'

export default function Exits() {
  const qc = useQueryClient()
  const { data: exits = [], isLoading } = useQuery({ queryKey: ['exits'], queryFn: getExits })
  const [showForm, setShowForm] = useState(false)
  const [logModal, setLogModal] = useState<{ exit: Exit; logs: string } | null>(null)
  const [editExit, setEditExit] = useState<Exit | null>(null)

  const invalidate = () => qc.invalidateQueries({ queryKey: ['exits'] })

  const createMut = useMutation({ mutationFn: createExit, onSuccess: () => { invalidate(); setShowForm(false) } })
  const deleteMut = useMutation({ mutationFn: deleteExit, onSuccess: invalidate })
  const startMut = useMutation({ mutationFn: startExit, onSuccess: invalidate })
  const stopMut = useMutation({ mutationFn: stopExit, onSuccess: invalidate })
  const restartMut = useMutation({ mutationFn: restartExit, onSuccess: invalidate })

  const handleLogs = async (exit: Exit) => {
    const res = await getExitLogs(exit.id)
    setLogModal({ exit, logs: res.logs })
  }

  return (
    <div className="space-y-5 max-w-5xl">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">出口管理</h1>
          <p className="text-sm text-gray-400 mt-1">管理 Gluetun VPN 出口容器</p>
        </div>
        <button className="btn-primary" onClick={() => setShowForm(true)}>
          <Plus size={16} /> 新增出口
        </button>
      </div>

      {isLoading ? (
        <div className="text-gray-500 text-sm">載入中...</div>
      ) : exits.length === 0 ? (
        <div className="card text-center py-12">
          <Globe size={32} className="mx-auto text-gray-600 mb-3" />
          <p className="text-gray-400">尚未設定出口</p>
          <button className="btn-primary mt-4" onClick={() => setShowForm(true)}>
            <Plus size={16} /> 新增第一個出口
          </button>
        </div>
      ) : (
        <div className="space-y-3">
          {exits.map(exit => (
            <ExitCard
              key={exit.id}
              exit={exit}
              onStart={() => startMut.mutate(exit.id)}
              onStop={() => stopMut.mutate(exit.id)}
              onRestart={() => restartMut.mutate(exit.id)}
              onDelete={() => {
                if (confirm(`確定刪除出口 "${exit.name}"？`)) deleteMut.mutate(exit.id)
              }}
              onLogs={() => handleLogs(exit)}
            />
          ))}
        </div>
      )}

      {/* Create Form Modal */}
      {showForm && (
        <ExitFormModal
          onClose={() => setShowForm(false)}
          onSubmit={(data) => createMut.mutate(data)}
          loading={createMut.isPending}
          error={createMut.error?.message}
        />
      )}

      {/* Logs Modal */}
      {logModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4">
          <div className="bg-gray-900 border border-gray-700 rounded-xl w-full max-w-3xl">
            <div className="flex items-center justify-between px-5 py-4 border-b border-gray-800">
              <h3 className="font-semibold text-white">Logs — {logModal.exit.name}</h3>
              <button className="btn-ghost p-1" onClick={() => setLogModal(null)}><X size={16} /></button>
            </div>
            <pre className="p-5 text-xs font-mono text-green-400 bg-black/50 overflow-auto max-h-96 whitespace-pre-wrap break-all">
              {logModal.logs || '(no logs)'}
            </pre>
          </div>
        </div>
      )}
    </div>
  )
}

function ExitCard({
  exit, onStart, onStop, onRestart, onDelete, onLogs
}: {
  exit: Exit
  onStart: () => void
  onStop: () => void
  onRestart: () => void
  onDelete: () => void
  onLogs: () => void
}) {
  return (
    <div className="card">
      <div className="flex items-start justify-between gap-4">
        <div className="flex items-center gap-3">
          <div className={`w-3 h-3 rounded-full flex-shrink-0 mt-0.5 ${
            exit.status === 'running' ? 'bg-green-400' :
            exit.status === 'unhealthy' ? 'bg-yellow-400' : 'bg-gray-600'
          }`} />
          <div>
            <div className="font-semibold text-white">{exit.name}</div>
            <div className="text-xs text-gray-400 mt-0.5">
              {exit.provider} · {exit.protocol} · {exit.country} {exit.city && `(${exit.city})`}
            </div>
            {exit.public_ip && (
              <div className="text-xs font-mono text-indigo-400 mt-0.5">{exit.public_ip}</div>
            )}
          </div>
        </div>
        <div className="flex items-center gap-1 flex-wrap justify-end">
          {exit.status !== 'running' ? (
            <button className="btn-secondary text-xs" onClick={onStart}><Play size={13} /> 啟動</button>
          ) : (
            <button className="btn-secondary text-xs" onClick={onStop}><Square size={13} /> 停止</button>
          )}
          <button className="btn-secondary text-xs" onClick={onRestart}><RotateCcw size={13} /> 重啟</button>
          <button className="btn-ghost text-xs" onClick={onLogs}><Terminal size={13} /> 日誌</button>
          <button className="btn-danger text-xs" onClick={onDelete}><Trash2 size={13} /></button>
        </div>
      </div>
    </div>
  )
}

function ExitFormModal({
  onClose, onSubmit, loading, error
}: {
  onClose: () => void
  onSubmit: (data: any) => void
  loading: boolean
  error?: string
}) {
  const [form, setForm] = useState({
    name: '',
    protocol: 'wireguard',
    provider: 'protonvpn',
    country: '',
    city: '',
    wireguard_private_key: '',
    wireguard_addresses: '',
    openvpn_user: '',
    openvpn_password: '',
  })

  const set = (key: string, val: string) => setForm(f => ({ ...f, [key]: val }))

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4">
      <div className="bg-gray-900 border border-gray-700 rounded-xl w-full max-w-lg">
        <div className="flex items-center justify-between px-5 py-4 border-b border-gray-800">
          <h3 className="font-semibold text-white">新增出口</h3>
          <button className="btn-ghost p-1" onClick={onClose}><X size={16} /></button>
        </div>
        <div className="p-5 space-y-4">
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="label">出口名稱 *</label>
              <input className="input" placeholder="us" value={form.name} onChange={e => set('name', e.target.value)} />
            </div>
            <div>
              <label className="label">VPN 協定</label>
              <select className="select" value={form.protocol} onChange={e => set('protocol', e.target.value)}>
                <option value="wireguard">WireGuard</option>
                <option value="openvpn">OpenVPN</option>
              </select>
            </div>
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="label">VPN 供應商</label>
              <input className="input" placeholder="protonvpn" value={form.provider} onChange={e => set('provider', e.target.value)} />
            </div>
            <div>
              <label className="label">國家</label>
              <input className="input" placeholder="United States" value={form.country} onChange={e => set('country', e.target.value)} />
            </div>
          </div>
          <div>
            <label className="label">城市（可選）</label>
            <input className="input" placeholder="New York" value={form.city} onChange={e => set('city', e.target.value)} />
          </div>

          {form.protocol === 'wireguard' ? (
            <>
              <div>
                <label className="label">WireGuard Private Key *</label>
                <input className="input font-mono" placeholder="私鑰" value={form.wireguard_private_key} onChange={e => set('wireguard_private_key', e.target.value)} />
              </div>
              <div>
                <label className="label">WireGuard Addresses（可選）</label>
                <input className="input font-mono" placeholder="10.2.0.2/32" value={form.wireguard_addresses} onChange={e => set('wireguard_addresses', e.target.value)} />
              </div>
            </>
          ) : (
            <>
              <div>
                <label className="label">OpenVPN 帳號</label>
                <input className="input" value={form.openvpn_user} onChange={e => set('openvpn_user', e.target.value)} />
              </div>
              <div>
                <label className="label">OpenVPN 密碼</label>
                <input className="input" type="password" value={form.openvpn_password} onChange={e => set('openvpn_password', e.target.value)} />
              </div>
            </>
          )}

          {error && <div className="text-red-400 text-sm">{error}</div>}
        </div>
        <div className="flex justify-end gap-2 px-5 py-4 border-t border-gray-800">
          <button className="btn-secondary" onClick={onClose}>取消</button>
          <button
            className="btn-primary"
            disabled={loading || !form.name}
            onClick={() => onSubmit(form)}
          >
            {loading ? '建立中...' : '建立出口'}
          </button>
        </div>
      </div>
    </div>
  )
}
