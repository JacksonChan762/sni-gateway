import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  getSubscriptions, createSubscription, updateSubscription,
  deleteSubscription, syncSubscription, getExits, getApiError,
} from '../api/client'
import { type Subscription } from '../api/client'
import {
  Plus, Trash2, RefreshCw, Pencil, X, Rss,
  CheckCircle, XCircle, Clock, ExternalLink,
} from 'lucide-react'
import { useToast } from '../components/Toast'
import { useConfirm } from '../components/ConfirmModal'

export default function Subscriptions() {
  const qc = useQueryClient()
  const toast = useToast()
  const { confirm, modal: confirmModal } = useConfirm()
  const { data: subs = [], isLoading, error, refetch } = useQuery({ queryKey: ['subscriptions'], queryFn: getSubscriptions, refetchInterval: 15_000 })
  const { data: exits = [] } = useQuery({ queryKey: ['exits'], queryFn: getExits })
  const [showForm, setShowForm] = useState(false)
  const [editSub, setEditSub] = useState<Subscription | null>(null)
  const [syncing, setSyncing] = useState<number | null>(null)

  const invalidate = () => qc.invalidateQueries({ queryKey: ['subscriptions'] })

  const createMut = useMutation({
    mutationFn: createSubscription,
    onSuccess: () => { invalidate(); setShowForm(false); toast.success('Subscription created') },
    onError: err => toast.error(getApiError(err)),
  })
  const updateMut = useMutation({
    mutationFn: ({ id, data }: { id: number; data: any }) => updateSubscription(id, data),
    onSuccess: () => { invalidate(); setEditSub(null); toast.success('Subscription updated') },
    onError: err => toast.error(getApiError(err)),
  })
  const deleteMut = useMutation({
    mutationFn: deleteSubscription,
    onSuccess: () => { invalidate(); toast.success('Subscription deleted') },
    onError: err => toast.error(getApiError(err)),
  })

  const handleSync = async (sub: Subscription) => {
    setSyncing(sub.id)
    try {
      await syncSubscription(sub.id)
      invalidate()
      toast.success(`"${sub.name}" synced successfully`)
    } catch (err) {
      toast.error(getApiError(err))
    } finally {
      setSyncing(null)
    }
  }

  const handleDelete = async (sub: Subscription) => {
    const ok = await confirm({
      title: 'Delete Subscription',
      message: `Delete "${sub.name}"? This will also remove ${sub.rule_count} associated rules.`,
      confirmText: 'Delete',
      dangerous: true,
    })
    if (ok) deleteMut.mutate(sub.id)
  }

  return (
    <div className="space-y-6">
      {confirmModal}

      <div className="page-header">
        <div>
          <h1 className="page-title">Subscriptions</h1>
          <p className="page-subtitle">Sync remote .list rule files automatically</p>
        </div>
        <button className="btn-primary" onClick={() => setShowForm(true)}>
          <Plus size={15} /> New Subscription
        </button>
      </div>

      <p className="text-sm text-gray-400">Enabled lists sync and publish routing changes automatically. Status refreshes every 15 seconds.</p>
      {error && <div role="alert" className="card border-red-800 text-red-300 text-sm">
        Could not refresh subscriptions: {getApiError(error)}
        <button className="btn-secondary ml-3" onClick={() => void refetch()}>Retry</button>
      </div>}
      {isLoading ? (
        <div className="card flex items-center justify-center py-16">
          <div className="text-gray-500 text-sm flex items-center gap-2">
            <svg className="animate-spin h-4 w-4" viewBox="0 0 24 24" fill="none">
              <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
              <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
            </svg>
            Loading subscriptions...
          </div>
        </div>
      ) : error && subs.length === 0 ? null : subs.length === 0 ? (
        <div className="card">
          <div className="empty-state">
            <div className="w-14 h-14 rounded-2xl bg-gray-800 flex items-center justify-center mb-4">
              <Rss size={24} className="text-gray-600" />
            </div>
            <h3 className="text-gray-300 font-medium mb-1">No subscriptions</h3>
            <p className="text-gray-500 text-sm mb-4">Subscribe to remote rule lists for automatic updates</p>
            <button className="btn-primary" onClick={() => setShowForm(true)}>
              <Plus size={15} /> Add Subscription
            </button>
          </div>
        </div>
      ) : (
        <div className="space-y-3">
          {subs.map(sub => (
            <SubCard
              key={sub.id}
              sub={sub}
              syncing={syncing === sub.id}
              onSync={() => handleSync(sub)}
              onEdit={() => setEditSub(sub)}
              onDelete={() => handleDelete(sub)}
            />
          ))}
        </div>
      )}

      {(showForm || editSub) && (
        <SubFormModal
          sub={editSub}
          exits={exits}
          onClose={() => { setShowForm(false); setEditSub(null) }}
          onSubmit={data => {
            if (editSub) updateMut.mutate({ id: editSub.id, data })
            else createMut.mutate(data)
          }}
          loading={createMut.isPending || updateMut.isPending}
          error={(createMut.error && getApiError(createMut.error)) || (updateMut.error && getApiError(updateMut.error)) || ''}
        />
      )}
    </div>
  )
}

function SubCard({
  sub, syncing, onSync, onEdit, onDelete,
}: {
  sub: Subscription
  syncing: boolean
  onSync: () => void
  onEdit: () => void
  onDelete: () => void
}) {
  const syncIcon = sub.last_sync_status === 'success'
    ? <CheckCircle size={11} className="text-green-400" />
    : sub.last_sync_status === 'failed'
    ? <XCircle size={11} className="text-red-400" />
    : <Clock size={11} className="text-gray-500" />

  return (
    <div className="card hover:border-gray-700/80 transition-colors">
      <div className="flex items-start justify-between gap-4">
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 mb-1 flex-wrap">
            <span className="font-semibold text-white">{sub.name}</span>
            <span className={sub.enabled ? 'badge-green' : 'badge-gray'}>
              {sub.enabled ? 'Active' : 'Disabled'}
            </span>
            {sub.exit && (
              <span className="badge-blue">→ {sub.exit.name}</span>
            )}
          </div>

          <div className="flex items-center gap-1.5 mb-2">
            <span className="text-xs font-mono text-gray-500 truncate max-w-md">{sub.url}</span>
            <a
              href={sub.url}
              target="_blank"
              rel="noopener noreferrer"
              className="text-gray-600 hover:text-gray-400 transition-colors flex-shrink-0"
              title="Open URL"
            >
              <ExternalLink size={11} />
            </a>
          </div>

          <div className="flex items-center gap-4 text-xs text-gray-500 flex-wrap">
            <span className="badge-gray">{sub.rule_count} rules</span>
            <span>{sub.enabled ? `Syncs every ${sub.interval_minutes} min` : 'Automatic sync paused'}</span>
            {sub.last_sync_at && (
              <span className="flex items-center gap-1">
                {syncIcon}
                Last sync: {new Date(sub.last_sync_at).toLocaleString()}
              </span>
            )}
          </div>

          {sub.last_error && (
            <div className="text-xs text-red-400 mt-2 bg-red-950/30 border border-red-800/30 px-2.5 py-1.5 rounded-lg truncate">
              {sub.last_error}
            </div>
          )}
        </div>

        <div className="flex items-center gap-1 flex-shrink-0">
          <button
            className="btn-secondary text-xs"
            disabled={syncing}
            onClick={onSync}
            title="Sync now"
          >
            <RefreshCw size={13} className={syncing ? 'animate-spin' : ''} />
            {syncing ? 'Syncing...' : 'Sync'}
          </button>
          <button className="btn-ghost p-1.5" onClick={onEdit} title="Edit">
            <Pencil size={13} />
          </button>
          <button
            className="btn-ghost p-1.5 text-red-500 hover:text-red-400 hover:bg-red-950/30"
            onClick={onDelete}
            title="Delete"
          >
            <Trash2 size={13} />
          </button>
        </div>
      </div>
    </div>
  )
}

function SubFormModal({
  sub, exits, onClose, onSubmit, loading, error,
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
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-sm">
      <div className="bg-gray-900 border border-gray-700 rounded-2xl w-full max-w-lg shadow-2xl">
        <div className="flex items-center justify-between px-5 py-4 border-b border-gray-800">
          <h3 className="font-semibold text-white">{sub ? 'Edit Subscription' : 'New Subscription'}</h3>
          <button className="btn-ghost p-1" onClick={onClose}><X size={16} /></button>
        </div>
        <div className="p-5 space-y-4">
          <div>
            <label className="label">Name *</label>
            <input
              className="input"
              placeholder="AI Sites"
              value={form.name}
              onChange={e => set('name', e.target.value)}
            />
          </div>
          <div>
            <label className="label">Subscription URL *</label>
            <input
              className="input font-mono text-xs"
              placeholder="https://example.com/rules.list"
              value={form.url}
              onChange={e => set('url', e.target.value)}
            />
            <p className="text-xs text-gray-600 mt-1">Must return a plain-text .list file</p>
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="label">Bind to Exit</label>
              <select
                className="select"
                value={form.exit_id ?? ''}
                onChange={e => set('exit_id', e.target.value ? parseInt(e.target.value) : null)}
              >
                <option value="">— None —</option>
                {exits.map(exit => (
                  <option key={exit.id} value={exit.id}>{exit.name}</option>
                ))}
              </select>
            </div>
            <div>
              <label className="label">Sync Interval (minutes)</label>
              <input
                className="input"
                type="number"
                min={5}
                value={form.interval_minutes}
                onChange={e => set('interval_minutes', parseInt(e.target.value) || 360)}
              />
            </div>
          </div>
          <label className="flex items-center gap-2 cursor-pointer select-none">
            <div
              className={`relative w-9 h-5 rounded-full transition-colors ${form.enabled ? 'bg-indigo-600' : 'bg-gray-700'}`}
              onClick={() => set('enabled', !form.enabled)}
            >
              <div className={`absolute top-0.5 w-4 h-4 rounded-full bg-white shadow transition-all ${form.enabled ? 'left-4' : 'left-0.5'}`} />
            </div>
            <span className="text-sm text-gray-300">Active</span>
          </label>
          {error && (
            <div className="text-red-400 text-sm bg-red-950/40 border border-red-800/50 px-3 py-2.5 rounded-xl">
              {error}
            </div>
          )}
        </div>
        <div className="flex justify-end gap-2 px-5 py-4 border-t border-gray-800">
          <button className="btn-secondary" onClick={onClose}>Cancel</button>
          <button
            className="btn-primary"
            disabled={loading || !form.name || !form.url}
            onClick={() => onSubmit(form)}
          >
            {loading ? 'Saving...' : sub ? 'Update' : 'Create Subscription'}
          </button>
        </div>
      </div>
    </div>
  )
}
