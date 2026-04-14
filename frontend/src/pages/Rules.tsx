import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { getRules, createRule, updateRule, deleteRule, getExits } from '../api/client'
import { type Rule } from '../api/client'
import { Plus, Trash2, Pencil, X, ListFilter } from 'lucide-react'

export default function Rules() {
  const qc = useQueryClient()
  const { data: rules = [], isLoading } = useQuery({ queryKey: ['rules'], queryFn: getRules })
  const { data: exits = [] } = useQuery({ queryKey: ['exits'], queryFn: getExits })
  const [showForm, setShowForm] = useState(false)
  const [editRule, setEditRule] = useState<Rule | null>(null)

  const invalidate = () => qc.invalidateQueries({ queryKey: ['rules'] })

  const createMut = useMutation({ mutationFn: createRule, onSuccess: () => { invalidate(); setShowForm(false) } })
  const updateMut = useMutation({ mutationFn: ({ id, data }: { id: number; data: Partial<Rule> }) => updateRule(id, data),
    onSuccess: () => { invalidate(); setEditRule(null) }
  })
  const deleteMut = useMutation({ mutationFn: deleteRule, onSuccess: invalidate })

  const manualRules = rules.filter(r => r.source === 'manual')
  const subRules = rules.filter(r => r.source === 'subscription')

  return (
    <div className="space-y-5 max-w-5xl">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">規則管理</h1>
          <p className="text-sm text-gray-400 mt-1">共 {rules.length} 條規則 · 手動 {manualRules.length} · 訂閱 {subRules.length}</p>
        </div>
        <button className="btn-primary" onClick={() => setShowForm(true)}>
          <Plus size={16} /> 新增規則
        </button>
      </div>

      {isLoading ? (
        <div className="text-gray-500 text-sm">載入中...</div>
      ) : rules.length === 0 ? (
        <div className="card text-center py-12">
          <ListFilter size={32} className="mx-auto text-gray-600 mb-3" />
          <p className="text-gray-400">尚未設定規則</p>
        </div>
      ) : (
        <div className="card overflow-hidden p-0">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-xs text-gray-500 border-b border-gray-800">
                <th className="px-4 py-3">Pattern</th>
                <th className="px-4 py-3">類型</th>
                <th className="px-4 py-3">出口</th>
                <th className="px-4 py-3">優先序</th>
                <th className="px-4 py-3">來源</th>
                <th className="px-4 py-3">狀態</th>
                <th className="px-4 py-3 text-right">操作</th>
              </tr>
            </thead>
            <tbody>
              {rules.map(rule => (
                <tr key={rule.id} className="table-row-hover border-b border-gray-800/50 last:border-0">
                  <td className="px-4 py-3 font-mono text-indigo-300">{rule.pattern}</td>
                  <td className="px-4 py-3">
                    <span className={`badge ${rule.type === 'suffix' ? 'badge-blue' : 'badge-gray'}`}>
                      {rule.type === 'suffix' ? '後綴' : '精確'}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-gray-300">{rule.exit?.name ?? <span className="text-gray-600">—</span>}</td>
                  <td className="px-4 py-3 text-gray-400">{rule.priority}</td>
                  <td className="px-4 py-3">
                    <span className={`badge ${rule.source === 'manual' ? 'badge-gray' : 'badge-blue'}`}>
                      {rule.source === 'manual' ? '手動' : '訂閱'}
                    </span>
                  </td>
                  <td className="px-4 py-3">
                    <span className={`badge ${rule.enabled ? 'badge-green' : 'badge-gray'}`}>
                      {rule.enabled ? '啟用' : '停用'}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-right">
                    <div className="flex items-center justify-end gap-1">
                      {rule.source === 'manual' && (
                        <button className="btn-ghost p-1" onClick={() => setEditRule(rule)}>
                          <Pencil size={13} />
                        </button>
                      )}
                      <button className="btn-ghost p-1 text-red-400 hover:text-red-300" onClick={() => {
                        if (confirm(`刪除規則 "${rule.pattern}"？`)) deleteMut.mutate(rule.id)
                      }}>
                        <Trash2 size={13} />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {(showForm || editRule) && (
        <RuleFormModal
          rule={editRule}
          exits={exits}
          onClose={() => { setShowForm(false); setEditRule(null) }}
          onSubmit={(data) => {
            if (editRule) {
              updateMut.mutate({ id: editRule.id, data })
            } else {
              createMut.mutate(data)
            }
          }}
          loading={createMut.isPending || updateMut.isPending}
          error={createMut.error?.message || updateMut.error?.message}
        />
      )}
    </div>
  )
}

function RuleFormModal({
  rule, exits, onClose, onSubmit, loading, error
}: {
  rule: Rule | null
  exits: any[]
  onClose: () => void
  onSubmit: (data: any) => void
  loading: boolean
  error?: string
}) {
  const [form, setForm] = useState({
    pattern: rule?.pattern ?? '',
    type: rule?.type ?? 'exact',
    exit_id: rule?.exit_id ?? null,
    priority: rule?.priority ?? 100,
    enabled: rule?.enabled ?? true,
  })

  const set = (key: string, val: any) => setForm(f => ({ ...f, [key]: val }))

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4">
      <div className="bg-gray-900 border border-gray-700 rounded-xl w-full max-w-md">
        <div className="flex items-center justify-between px-5 py-4 border-b border-gray-800">
          <h3 className="font-semibold text-white">{rule ? '編輯規則' : '新增規則'}</h3>
          <button className="btn-ghost p-1" onClick={onClose}><X size={16} /></button>
        </div>
        <div className="p-5 space-y-4">
          <div>
            <label className="label">Pattern *</label>
            <input
              className="input font-mono"
              placeholder="example.com 或 +.example.com"
              value={form.pattern}
              onChange={e => {
                const v = e.target.value
                set('pattern', v)
                if (v.startsWith('+.')) set('type', 'suffix')
                else set('type', 'exact')
              }}
            />
            <p className="text-xs text-gray-500 mt-1">
              前綴 +. 表示後綴匹配（含子網域）
            </p>
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="label">類型</label>
              <select className="select" value={form.type} onChange={e => set('type', e.target.value)}>
                <option value="exact">精確匹配</option>
                <option value="suffix">後綴匹配</option>
              </select>
            </div>
            <div>
              <label className="label">優先序</label>
              <input className="input" type="number" value={form.priority} onChange={e => set('priority', parseInt(e.target.value) || 100)} />
            </div>
          </div>
          <div>
            <label className="label">出口</label>
            <select className="select" value={form.exit_id ?? ''} onChange={e => set('exit_id', e.target.value ? parseInt(e.target.value) : null)}>
              <option value="">— 未指定 —</option>
              {exits.map(exit => (
                <option key={exit.id} value={exit.id}>{exit.name} ({exit.country})</option>
              ))}
            </select>
          </div>
          <div className="flex items-center gap-2">
            <input type="checkbox" id="enabled" checked={form.enabled} onChange={e => set('enabled', e.target.checked)} className="rounded" />
            <label htmlFor="enabled" className="text-sm text-gray-300">啟用規則</label>
          </div>
          {error && <div className="text-red-400 text-sm">{error}</div>}
        </div>
        <div className="flex justify-end gap-2 px-5 py-4 border-t border-gray-800">
          <button className="btn-secondary" onClick={onClose}>取消</button>
          <button
            className="btn-primary"
            disabled={loading || !form.pattern}
            onClick={() => onSubmit(form)}
          >
            {loading ? '儲存中...' : rule ? '更新規則' : '建立規則'}
          </button>
        </div>
      </div>
    </div>
  )
}
