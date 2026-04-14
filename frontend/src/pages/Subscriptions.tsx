import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  getSubscriptions, createSubscription, updateSubscription,
  deleteSubscription, syncSubscription, getExits
} from '../api/client'
import { type Subscription } from '../api/client'
import { Plus, Trash2, RefreshCw, Pencil, X, Rss, CheckCircle, XCircle, Clock } from 'lucide-react'

export default function Subscriptions() {
  const qc = useQueryClient()
  const { data: subs = [], isLoading } = useQuery({ queryKey: ['subscriptions'], queryFn: getSubscriptions })
  const { data: exits = [] } = useQuery({ queryKey: ['exits'], queryFn: getExits })
  const [showForm, setShowForm] = useState(false)
  const [editSub, setEditSub] = useState<Subscription | null>(null)
  const [syncing, setSyncing] = useState<number | null>(null)

  const invalidate = () => qc.invalidateQueries({ queryKey: ['subscriptions'] })

  const createMut = useMutation({ mutationFn: createSubscription, onSuccess: () => { invalidate(); setShowForm(false) } })
  const updateMut = useMutation({
    mutationFn: ({ id, data }: { id: number; data: any }) => updateSubscription(id, data),
    onSuccess: () => { invalidate(); setEditSub(null) }
  })
  const deleteMut = useMutation({ mutationFn: deleteSubscription, onSuccess: invalidate })

  const handleSync = async (id: number) => {
    setSyncing(id)
    try {
      await syncSubscription(id)
      invalidate()
    } finally {
      setSyncing(null)
    }
  }

  return (
    <div className="space-y-5 max-w-5xl">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">訂閱管理</h1>
          <p className="text-sm text-gray-400 mt-1">管理遠端 .list 規則訂閱</p>
        </div>
        <button className="btn-primary" onClick={() => setShowForm(true)}>
          <Plus size={16} /> 新增訂閱
        </button>
      </div>

      {isLoading ? (
        <div className="text-gray-500 text-sm">載入中...</div>
      ) : subs.length === 0 ? (
        <div className="card text-center py-12">
          <Rss size={32} className="mx-auto text-gray-600 mb-3" />
          <p className="text-gray-400">尚未新增訂閱</p>
        </div>
      ) : (
        <div className="space-y-3">
          {subs.map(sub => (
            <SubCard
              key={sub.id}
              sub={sub}
              syncing={syncing === sub.id}
              onSync={() => handleSync(sub.id)}
              onEdit={() => setEditSub(sub)}
              onDelete={() => {
                if (confirm(`刪除訂閱 "${sub.name}"？\n這將同時刪除 ${sub.rule_count} 條相關規則。`))
                  deleteMut.mutate(sub.id)
              }}
            />
          ))}
        </div>
      )}

      {(showForm || editSub) && (
        <SubFormModal
          sub={editSub}
          exits={exits}
          onClose={() => { setShowForm(false); setEditSub(null) }}
          onSubmit={(data) => {
            if (editSub) updateMut.mutate({ id: editSub.id, data })
            else createMut.mutate(data)
          }}
          loading={createMut.isPending || updateMut.isPending}
          error={createMut.error?.message || updateMut.error?.message}
        />
      )}
    </div>
  )
}

function SubCard({
  sub, syncing, onSync, onEdit, onDelete
}: {
  sub: Subscription
  syncing: boolean
  onSync: () => void
  onEdit: () => void
  onDelete: () => void
}) {
  return (
    <div className="card">
      <div className="flex items-start justify-between gap-4">
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 mb-1">
            <span className="font-semibold text-white">{sub.name}</span>
            <span className={`badge ${sub.enabled ? 'badge-green' : 'badge-gray'}`}>
              {sub.enabled ? '啟用' : '停用'}
            </span>
            {sub.exit && <span className="badge badge-blue">→ {sub.exit.name}</span>}
          </div>
          <div className="text-xs font-mono text-gray-400 truncate mb-2">{sub.url}</div>
          <div className="flex items-center gap-4 text-xs text-gray-500">
            <span>{sub.rule_count} 條規則</span>
            <span>每 {sub.interval_minutes} 分鐘更新</span>
            {sub.last_sync_at && (
              <span className="flex items-center gap-1">
                {sub.last_sync_status === 'success'
                  ? <CheckCircle size={11} className="text-green-400" />
                  : sub.last_sync_status === 'failed'
                  ? <XCircle size={11} className="text-red-400" />
                  : <Clock size={11} />
                }
                {new Date(sub.last_sync_at).toLocaleString('zh-TW')}
              </span>
            )}
          </div>
          {sub.last_error && (
            <div className="text-xs text-red-400 mt-1 truncate">{sub.last_error}</div>
          )}
        </div>
        <div className="flex items-center gap-1 flex-shrink-0">
          <button
            className="btn-secondary text-xs"
            disabled={syncing}
            onClick={onSync}
          >
            <RefreshCw size={13} className={syncing ? 'animate-spin' : ''} />
            {syncing ? '同步中...' : '立即同步'}
          </button>
          <button className="btn-ghost p-1.5" onClick={onEdit}><Pencil size={13} /></button>
          <button className="btn-ghost p-1.5 text-red-400 hover:text-red-300" onClick={onDelete}><Trash2 size={13} /></button>
        </div>
      </div>
    </div>
  )
}

function SubFormModal({
  sub, exits, onClose, onSubmit, loading, error
}: {
  sub: Subscription | null
  exits: any[]
  onClose: () => void
  onSubmit: (data: any) => void
  loading: boolean
  error?: string
}) {
  const [form, setForm] = useState({
    name: sub?.name ?? '',
    url: sub?.url ?? '',
    exit_id: sub?.exit_id ?? null,
    enabled: sub?.enabled ?? true,
    interval_minutes: sub?.interval_minutes ?? 360,
  })

  const set = (key: string, val: any) => setForm(f => ({ ...f, [key]: val }))

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4">
      <div className="bg-gray-900 border border-gray-700 rounded-xl w-full max-w-lg">
        <div className="flex items-center justify-between px-5 py-4 border-b border-gray-800">
          <h3 className="font-semibold text-white">{sub ? '編輯訂閱' : '新增訂閱'}</h3>
          <button className="btn-ghost p-1" onClick={onClose}><X size={16} /></button>
        </div>
        <div className="p-5 space-y-4">
          <div>
            <label className="label">名稱 *</label>
            <input className="input" placeholder="AI Sites" value={form.name} onChange={e => set('name', e.target.value)} />
          </div>
          <div>
            <label className="label">訂閱 URL *</label>
            <input className="input font-mono text-xs" placeholder="https://example.com/rules.list" value={form.url} onChange={e => set('url', e.target.value)} />
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="label">綁定出口</label>
              <select className="select" value={form.exit_id ?? ''} onChange={e => set('exit_id', e.target.value ? parseInt(e.target.value) : null)}>
                <option value="">— 未指定 —</option>
                {exits.map(exit => (
                  <option key={exit.id} value={exit.id}>{exit.name}</option>
                ))}
              </select>
            </div>
            <div>
              <label className="label">更新週期（分鐘）</label>
              <input className="input" type="number" min={5} value={form.interval_minutes} onChange={e => set('interval_minutes', parseInt(e.target.value) || 360)} />
            </div>
          </div>
          <div className="flex items-center gap-2">
            <input type="checkbox" id="sub-enabled" checked={form.enabled} onChange={e => set('enabled', e.target.checked)} />
            <label htmlFor="sub-enabled" className="text-sm text-gray-300">啟用訂閱</label>
          </div>
          {error && <div className="text-red-400 text-sm">{error}</div>}
        </div>
        <div className="flex justify-end gap-2 px-5 py-4 border-t border-gray-800">
          <button className="btn-secondary" onClick={onClose}>取消</button>
          <button
            className="btn-primary"
            disabled={loading || !form.name || !form.url}
            onClick={() => onSubmit(form)}
          >
            {loading ? '儲存中...' : sub ? '更新訂閱' : '新增訂閱'}
          </button>
        </div>
      </div>
    </div>
  )
}
