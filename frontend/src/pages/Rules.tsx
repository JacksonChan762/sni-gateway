import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { getRules, createRule, createRulesBulk, updateRule, deleteRule, getExits, getApiError } from '../api/client'
import { type Rule } from '../api/client'
import { Plus, Trash2, Pencil, X, ListFilter, Search } from 'lucide-react'
import { useToast } from '../components/Toast'
import { useConfirm } from '../components/ConfirmModal'

type RuleFormType = Rule['type'] | 'auto'

export default function Rules() {
  const qc = useQueryClient()
  const toast = useToast()
  const { confirm, modal: confirmModal } = useConfirm()
  const { data: rules = [], isLoading } = useQuery({ queryKey: ['rules'], queryFn: getRules })
  const { data: exits = [] } = useQuery({ queryKey: ['exits'], queryFn: getExits })
  const [showForm, setShowForm] = useState(false)
  const [editRule, setEditRule] = useState<Rule | null>(null)
  const [search, setSearch] = useState('')
  const [filterSource, setFilterSource] = useState<'all' | 'manual' | 'subscription'>('all')

  const invalidate = () => qc.invalidateQueries({ queryKey: ['rules'] })

  const createMut = useMutation({
    mutationFn: createRule,
    onSuccess: () => { invalidate(); setShowForm(false); toast.success('Rule created') },
    onError: err => toast.error(getApiError(err)),
  })
  const createBulkMut = useMutation({
    mutationFn: createRulesBulk,
    onSuccess: data => {
      invalidate()
      setShowForm(false)
      toast.success(`${data.created_count} rules created`)
    },
    onError: err => toast.error(getApiError(err)),
  })
  const updateMut = useMutation({
    mutationFn: ({ id, data }: { id: number; data: Partial<Rule> }) => updateRule(id, data),
    onSuccess: () => { invalidate(); setEditRule(null); toast.success('Rule updated') },
    onError: err => toast.error(getApiError(err)),
  })
  const deleteMut = useMutation({
    mutationFn: deleteRule,
    onSuccess: () => { invalidate(); toast.success('Rule deleted') },
    onError: err => toast.error(getApiError(err)),
  })

  const handleDelete = async (rule: Rule) => {
    const ok = await confirm({
      title: 'Delete Rule',
      message: `Delete rule "${rule.pattern}"? This action cannot be undone.`,
      confirmText: 'Delete',
      dangerous: true,
    })
    if (ok) deleteMut.mutate(rule.id)
  }

  const manualCount = rules.filter(r => r.source === 'manual').length
  const subCount = rules.filter(r => r.source === 'subscription').length

  const filtered = rules.filter(r => {
    const matchSearch = !search || r.pattern.toLowerCase().includes(search.toLowerCase()) ||
      r.exit?.name?.toLowerCase().includes(search.toLowerCase())
    const matchSource = filterSource === 'all' || r.source === filterSource
    return matchSearch && matchSource
  })

  return (
    <div className="space-y-6">
      {confirmModal}

      <div className="page-header">
        <div>
          <h1 className="page-title">Rules</h1>
          <p className="page-subtitle">
            {rules.length} rules · {manualCount} manual · {subCount} from subscriptions
          </p>
        </div>
        <button className="btn-primary" onClick={() => setShowForm(true)}>
          <Plus size={15} /> New Rule
        </button>
      </div>

      {/* Filters */}
      {rules.length > 0 && (
        <div className="flex items-center gap-3 flex-wrap">
          <div className="relative flex-1 min-w-48">
            <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500" />
            <input
              className="input pl-8 text-sm"
              placeholder="Search by pattern or exit..."
              value={search}
              onChange={e => setSearch(e.target.value)}
            />
          </div>
          <div className="flex gap-1">
            {(['all', 'manual', 'subscription'] as const).map(s => (
              <button
                key={s}
                onClick={() => setFilterSource(s)}
                className={`px-3 py-1.5 rounded-lg text-xs font-medium transition-colors capitalize ${
                  filterSource === s
                    ? 'bg-indigo-600 text-white'
                    : 'bg-gray-800 text-gray-400 hover:text-gray-200'
                }`}
              >
                {s}
              </button>
            ))}
          </div>
        </div>
      )}

      {isLoading ? (
        <div className="card flex items-center justify-center py-16">
          <div className="text-gray-500 text-sm flex items-center gap-2">
            <svg className="animate-spin h-4 w-4" viewBox="0 0 24 24" fill="none">
              <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
              <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
            </svg>
            Loading rules...
          </div>
        </div>
      ) : rules.length === 0 ? (
        <div className="card">
          <div className="empty-state">
            <div className="w-14 h-14 rounded-2xl bg-gray-800 flex items-center justify-center mb-4">
              <ListFilter size={24} className="text-gray-600" />
            </div>
            <h3 className="text-gray-300 font-medium mb-1">No rules yet</h3>
            <p className="text-gray-500 text-sm mb-4">Rules map domain patterns to VPN exits</p>
            <button className="btn-primary" onClick={() => setShowForm(true)}>
              <Plus size={15} /> Add First Rule
            </button>
          </div>
        </div>
      ) : filtered.length === 0 ? (
        <div className="card text-center py-10">
          <p className="text-gray-500 text-sm">No rules match your search</p>
        </div>
      ) : (
        <div className="card p-0 overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-gray-800/80">
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500">Pattern</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500">Type</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500">Exit</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500">Priority</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500">Source</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500">Status</th>
                  <th className="px-4 py-3 text-right text-xs font-medium text-gray-500">Actions</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map(rule => (
                  <tr
                    key={rule.id}
                    className="border-b border-gray-800/40 last:border-0 table-row-hover"
                  >
                    <td className="px-4 py-3 font-mono text-indigo-300 text-xs">{rule.pattern}</td>
                    <td className="px-4 py-3">
                      <span className={rule.type === 'suffix' ? 'badge-blue' : 'badge-gray'}>
                        {rule.type === 'suffix' ? 'Suffix' : 'Exact'}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-gray-300 text-xs">
                      {rule.exit?.name ?? <span className="text-gray-600">—</span>}
                    </td>
                    <td className="px-4 py-3 text-gray-500 text-xs tabular-nums">{rule.priority}</td>
                    <td className="px-4 py-3">
                      <span className={rule.source === 'manual' ? 'badge-gray' : 'badge-blue'}>
                        {rule.source === 'manual' ? 'Manual' : 'Subscription'}
                      </span>
                    </td>
                    <td className="px-4 py-3">
                      <span className={rule.enabled ? 'badge-green' : 'badge-gray'}>
                        {rule.enabled ? 'Active' : 'Disabled'}
                      </span>
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex items-center justify-end gap-1">
                        {rule.source === 'manual' && (
                          <button
                            className="btn-ghost p-1.5"
                            onClick={() => setEditRule(rule)}
                            title="Edit"
                          >
                            <Pencil size={13} />
                          </button>
                        )}
                        <button
                          className="btn-ghost p-1.5 text-red-500 hover:text-red-400 hover:bg-red-950/30"
                          onClick={() => handleDelete(rule)}
                          title="Delete"
                        >
                          <Trash2 size={13} />
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {filtered.length < rules.length && (
            <div className="px-4 py-2 text-xs text-gray-600 border-t border-gray-800/50">
              Showing {filtered.length} of {rules.length} rules
            </div>
          )}
        </div>
      )}

      {(showForm || editRule) && (
        <RuleFormModal
          rule={editRule}
          exits={exits}
          onClose={() => { setShowForm(false); setEditRule(null) }}
          onSubmit={data => {
            if (editRule) updateMut.mutate({ id: editRule.id, data })
            else if (Array.isArray(data.rules)) createBulkMut.mutate(data.rules)
            else createMut.mutate(data)
          }}
          loading={createMut.isPending || createBulkMut.isPending || updateMut.isPending}
          error={(createMut.error && getApiError(createMut.error)) || (createBulkMut.error && getApiError(createBulkMut.error)) || (updateMut.error && getApiError(updateMut.error)) || ''}
        />
      )}
    </div>
  )
}

function RuleFormModal({
  rule, exits, onClose, onSubmit, loading, error,
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
    type: (rule?.type ?? 'auto') as RuleFormType,
    exit_id: rule?.exit_id ?? null,
    priority: rule?.priority ?? 100,
    enabled: rule?.enabled ?? true,
  })
  const [localError, setLocalError] = useState('')

  const set = (key: string, val: any) => setForm(f => ({ ...f, [key]: val }))
  const patterns = rule ? [form.pattern.trim()].filter(Boolean) : parseRulePatterns(form.pattern)

  const handleSubmit = () => {
    if (patterns.length === 0) {
      setLocalError('At least one pattern is required')
      return
    }
    setLocalError('')

    if (rule) {
      onSubmit({
        ...form,
        type: resolveRuleType(form.pattern, form.type),
      })
      return
    }

    const rules = patterns.map(pattern => ({
      pattern,
      type: resolveRuleType(pattern, form.type),
      exit_id: form.exit_id,
      priority: form.priority,
      enabled: form.enabled,
    }))
    onSubmit(rules.length === 1 ? rules[0] : { rules })
  }

  const displayError = localError || error

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-sm">
      <div className="bg-gray-900 border border-gray-700 rounded-2xl w-full max-w-md shadow-2xl">
        <div className="flex items-center justify-between px-5 py-4 border-b border-gray-800">
          <h3 className="font-semibold text-white">{rule ? 'Edit Rule' : 'New Rule'}</h3>
          <button className="btn-ghost p-1" onClick={onClose}><X size={16} /></button>
        </div>
        <div className="p-5 space-y-4">
          <div>
            <label className="label">{rule ? 'Pattern *' : 'Patterns *'}</label>
            {rule ? (
              <input
                className="input font-mono"
                placeholder="example.com or +.example.com"
                value={form.pattern}
                onChange={e => {
                  const v = e.target.value
                  set('pattern', v)
                }}
              />
            ) : (
              <textarea
                className="input font-mono min-h-[150px] resize-y"
                placeholder={'example.com\n+.example.org\napi.example.net'}
                value={form.pattern}
                onChange={e => {
                  set('pattern', e.target.value)
                }}
              />
            )}
            <div className="flex items-center justify-between gap-3 mt-1">
              <p className="text-xs text-gray-600">
                Auto detects +. prefixes as suffix matches
              </p>
              {!rule && (
                <span className="text-xs text-gray-500 tabular-nums">
                  {patterns.length} {patterns.length === 1 ? 'rule' : 'rules'}
                </span>
              )}
            </div>
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="label">Match Type</label>
              <select
                className="select"
                value={form.type}
                onChange={e => set('type', e.target.value as RuleFormType)}
              >
                {!rule && <option value="auto">Auto detect</option>}
                <option value="exact">Exact match</option>
                <option value="suffix">Suffix match</option>
              </select>
            </div>
            <div>
              <label className="label">Priority</label>
              <input
                className="input"
                type="number"
                min={1}
                max={1000}
                value={form.priority}
                onChange={e => set('priority', parseInt(e.target.value) || 100)}
              />
            </div>
          </div>
          <div>
            <label className="label">Exit</label>
            <select
              className="select"
              value={form.exit_id ?? ''}
              onChange={e => set('exit_id', e.target.value ? parseInt(e.target.value) : null)}
            >
              <option value="">— None —</option>
              {exits.map(exit => (
                <option key={exit.id} value={exit.id}>
                  {exit.name} ({exit.country})
                </option>
              ))}
            </select>
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
          {displayError && (
            <div className="text-red-400 text-sm bg-red-950/40 border border-red-800/50 px-3 py-2.5 rounded-xl">
              {displayError}
            </div>
          )}
        </div>
        <div className="flex justify-end gap-2 px-5 py-4 border-t border-gray-800">
          <button className="btn-secondary" onClick={onClose}>Cancel</button>
          <button
            className="btn-primary"
            disabled={loading || patterns.length === 0}
            onClick={handleSubmit}
          >
            {loading ? 'Saving...' : rule ? 'Update Rule' : patterns.length > 1 ? `Create ${patterns.length} Rules` : 'Create Rule'}
          </button>
        </div>
      </div>
    </div>
  )
}

function parseRulePatterns(input: string) {
  const seen = new Set<string>()
  const patterns: string[] = []

  for (const raw of input.split(/[\n,\s]+/)) {
    const trimmed = raw.trim()
    if (!trimmed || trimmed.startsWith('#')) continue
    if (seen.has(trimmed)) continue
    seen.add(trimmed)
    patterns.push(trimmed)
  }

  return patterns
}

function resolveRuleType(pattern: string, selectedType: RuleFormType): Rule['type'] {
  if (pattern.trimStart().startsWith('+.')) return 'suffix'
  if (selectedType === 'suffix') return 'suffix'
  return 'exact'
}
