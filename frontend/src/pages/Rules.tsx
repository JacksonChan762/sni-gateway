import { useEffect, useId, useRef, useState } from 'react'
import { keepPreviousData, useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  getRulesPage, createRule, createRulesBulk, updateRule, deleteRule, getExits, getApiError,
  getRuleFolders, createRuleFolder, renameRuleFolder, deleteRuleFolder,
  type Exit, type Rule, type RuleFolder,
} from '../api/client'
import { Plus, Trash2, Pencil, X, ListFilter, Search, FolderPlus } from 'lucide-react'
import { useToast } from '../components/Toast'
import { useConfirm } from '../components/ConfirmModal'
import PublishStatusNotice from '../components/PublishStatusNotice'

type RuleFormType = Rule['type'] | 'auto'
type FolderFilter = number | 'all' | 'unfiled'
type RuleFormValues = Pick<Rule, 'pattern' | 'type' | 'exit_id' | 'folder_id' | 'priority' | 'enabled'>
type RuleSubmission = Partial<RuleFormValues> | { rules: RuleFormValues[] }

export default function Rules() {
  const qc = useQueryClient()
  const toast = useToast()
  const { confirm, modal: confirmModal } = useConfirm()
  const [page, setPage] = useState(1)
  const [search, setSearch] = useState('')
  const [querySearch, setQuerySearch] = useState('')
  const [folder, setFolder] = useState<FolderFilter>('all')
  const [folderEditor, setFolderEditor] = useState<RuleFolder | 'new' | null>(null)
  const foldersQuery = useQuery({ queryKey: ['rule-folders'], queryFn: getRuleFolders })
  const folders = foldersQuery.data ?? []
  const selectedFolder = folders.find(item => item.id === folder)
  const folderNames = new Map(folders.map(item => [item.id, item.name]))
  useEffect(() => {
    const timer = window.setTimeout(() => setQuerySearch(search), 250)
    return () => window.clearTimeout(timer)
  }, [search])
  useEffect(() => {
    if (typeof folder === 'number' && foldersQuery.data && !selectedFolder) {
      setFolder('all')
      setPage(1)
    }
  }, [folder, foldersQuery.data, selectedFolder])
  const { data, isLoading, isFetching, isPlaceholderData, error, refetch } = useQuery({
    queryKey: ['rules', page, querySearch, folder],
    queryFn: () => getRulesPage(page, querySearch, folder === 'all' ? undefined : folder),
    placeholderData: keepPreviousData,
  })
  const rules = isPlaceholderData ? [] : data?.rules ?? []
  const total = data?.total ?? 0
  const pages = Math.max(1, Math.ceil(total / 50))
  useEffect(() => {
    if (!isFetching && page > pages) setPage(pages)
  }, [isFetching, page, pages])
  const { data: exits = [] } = useQuery({ queryKey: ['exits'], queryFn: getExits })
  const [showForm, setShowForm] = useState(false)
  const [editRule, setEditRule] = useState<Rule | null>(null)

  const invalidate = (routingChanged = true) => {
    void qc.invalidateQueries({ queryKey: ['rules'] })
    if (routingChanged) void qc.invalidateQueries({ queryKey: ['publish-status'] })
  }

  const createMut = useMutation({
    mutationFn: createRule,
    onSuccess: () => { invalidate(); setShowForm(false); toast.success('Rule saved. Publish to update live routing.') },
    onError: err => toast.error(getApiError(err)),
  })
  const createBulkMut = useMutation({
    mutationFn: createRulesBulk,
    onSuccess: data => {
      invalidate()
      setShowForm(false)
      toast.success(`${data.created_count} rules saved. Publish to update live routing.`)
    },
    onError: err => toast.error(getApiError(err)),
  })
  const updateMut = useMutation({
    mutationFn: ({ id, data }: { id: number; data: Partial<Rule> }) => updateRule(id, data),
    onSuccess: (_, { data }) => {
      const folderOnly = Object.keys(data).length === 1 && 'folder_id' in data
      invalidate(!folderOnly)
      setEditRule(null)
      toast.success(folderOnly ? 'Rule moved' : 'Rule saved. Publish to update live routing.')
    },
    onError: err => toast.error(getApiError(err)),
  })
  const deleteMut = useMutation({
    mutationFn: deleteRule,
    onSuccess: () => { invalidate(); toast.success('Rule deleted. Publish to update live routing.') },
    onError: err => toast.error(getApiError(err)),
  })

  const selectFolder = (value: FolderFilter) => {
    setFolder(value)
    setPage(1)
  }

  const saveFolder = useMutation({
    mutationFn: ({ name, id }: { name: string; id?: number }) =>
      id === undefined ? createRuleFolder(name) : renameRuleFolder(id, name),
    onSuccess: (saved, { id }) => {
      qc.setQueryData<RuleFolder[]>(['rule-folders'], current =>
        [...(current ?? []).filter(item => item.id !== saved.id), saved]
          .sort((a, b) => a.name.localeCompare(b.name)))
      void qc.invalidateQueries({ queryKey: ['rule-folders'] })
      setFolderEditor(null)
      if (id === undefined) {
        setSearch('')
        setQuerySearch('')
        selectFolder(saved.id)
      }
      toast.success(id === undefined ? 'Folder created' : 'Folder renamed')
    },
  })

  const deleteFolder = useMutation({
    mutationFn: deleteRuleFolder,
    onSuccess: (_, id) => {
      if (folder === id) selectFolder('unfiled')
      qc.setQueryData<RuleFolder[]>(['rule-folders'], current => current?.filter(item => item.id !== id))
      void qc.invalidateQueries({ queryKey: ['rule-folders'] })
      invalidate(false)
      toast.success('Folder deleted. Its rules are now unfiled.')
    },
    onError: err => toast.error(getApiError(err)),
  })

  const openFolderEditor = (value: RuleFolder | 'new') => {
    saveFolder.reset()
    setFolderEditor(value)
  }

  const handleDeleteFolder = async (item: RuleFolder) => {
    const ok = await confirm({
      title: 'Delete folder',
      message: `Delete "${item.name}"? Its rules will move to Unfiled and keep their routing settings.`,
      confirmText: 'Delete folder',
      dangerous: true,
    })
    if (ok) deleteFolder.mutate(item.id)
  }

  const handleDelete = async (rule: Rule) => {
    const ok = await confirm({
      title: 'Delete Rule',
      message: `Delete rule "${rule.pattern}"? This action cannot be undone.`,
      confirmText: 'Delete',
      dangerous: true,
    })
    if (ok) deleteMut.mutate(rule.id)
  }

  return (
    <div className="space-y-6">
      {confirmModal}

      <div className="page-header">
        <div>
          <h1 className="page-title">Rules</h1>
          <p className="page-subtitle">
            {total} {querySearch ? 'matching' : 'manual'} {total === 1 ? 'rule' : 'rules'}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <button className="btn-secondary" onClick={() => openFolderEditor('new')}>
            <FolderPlus size={15} aria-hidden="true" /> New folder
          </button>
          <button className="btn-primary" onClick={() => setShowForm(true)}>
            <Plus size={15} aria-hidden="true" /> New Rule
          </button>
        </div>
      </div>

      <PublishStatusNotice />
      {error && <div role="alert" className="card border-red-800 text-red-300 text-sm">
        Could not load rules: {getApiError(error)}
        <button className="btn-secondary ml-3" onClick={() => void refetch()}>Retry</button>
        {rules.length > 0 && <p className="mt-2">Showing the last received list.</p>}
      </div>}
      {foldersQuery.isError && <div role="alert" className="card border-red-800 text-red-300 text-sm">
        Could not load folders. {getApiError(foldersQuery.error)}
        <button className="btn-secondary ml-3" onClick={() => void foldersQuery.refetch()}>Retry folders</button>
      </div>}

      <div className="flex flex-wrap items-end gap-3">
        <div className="w-full sm:w-52">
          <label className="label" htmlFor="rule-folder-filter">Folder</label>
          <select id="rule-folder-filter" className="select" value={folder}
            onChange={event => {
              const value = event.target.value
              selectFolder(value === 'all' || value === 'unfiled' ? value : Number(value))
            }}>
            <option value="all">All folders</option>
            <option value="unfiled">Unfiled</option>
            {folders.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}
          </select>
        </div>
        <div className="min-w-48 flex-1">
          <label className="label" htmlFor="rule-search">Search rules</label>
          <div className="relative">
            <Search size={14} aria-hidden="true" className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-500" />
            <input
              id="rule-search"
              className="input pl-8 text-sm"
              type="search"
              placeholder="Pattern or exit"
              value={search}
              onChange={e => { setSearch(e.target.value); setPage(1) }}
            />
          </div>
        </div>
        {selectedFolder && <div className="flex flex-wrap gap-2" role="group" aria-label={`Actions for ${selectedFolder.name}`}>
          <button className="btn-secondary" disabled={deleteFolder.isPending} onClick={() => openFolderEditor(selectedFolder)}>
            <Pencil size={13} aria-hidden="true" /> Rename folder
          </button>
          <button className="btn-ghost text-red-400" disabled={deleteFolder.isPending} onClick={() => void handleDeleteFolder(selectedFolder)}>
            <Trash2 size={13} aria-hidden="true" /> {deleteFolder.isPending ? 'Deleting…' : 'Delete folder'}
          </button>
        </div>}
      </div>

      {isLoading || isPlaceholderData ? (
        <div className="card flex items-center justify-center py-16">
          <div className="text-gray-500 text-sm flex items-center gap-2">
            <svg className="animate-spin h-4 w-4" viewBox="0 0 24 24" fill="none">
              <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
              <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
            </svg>
            Loading rules...
          </div>
        </div>
      ) : error && rules.length === 0 ? null : total === 0 && querySearch === '' && folder === 'all' ? (
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
      ) : rules.length === 0 ? (
        <div className="card text-center py-10">
          <p className="text-gray-400 text-sm">{querySearch ? 'No rules match your search' : 'No rules in this folder'}</p>
          {!querySearch && <button className="btn-primary mt-3" onClick={() => setShowForm(true)}>
            <Plus size={15} aria-hidden="true" /> New Rule
          </button>}
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
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500">Folder</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500">Priority</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500">Status</th>
                  <th className="px-4 py-3 text-right text-xs font-medium text-gray-500">Actions</th>
                </tr>
              </thead>
              <tbody>
                {rules.map(rule => (
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
                    <td className="px-4 py-3 text-xs text-gray-400 [overflow-wrap:anywhere]">
                      {rule.folder_id == null ? 'Unfiled' : folderNames.get(rule.folder_id) ?? '—'}
                    </td>
                    <td className="px-4 py-3 text-gray-500 text-xs tabular-nums">{rule.priority}</td>
                    <td className="px-4 py-3">
                      <span className={rule.enabled ? 'badge-green' : 'badge-gray'}>
                        {rule.enabled ? 'Enabled' : 'Disabled'}
                      </span>
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex items-center justify-end gap-1">
                        <button
                          className="btn-ghost p-1.5"
                          onClick={() => setEditRule(rule)}
                          title="Edit"
                          aria-label={`Edit ${rule.pattern}`}
                        >
                          <Pencil size={13} />
                        </button>
                        <button
                          className="btn-ghost p-1.5 text-red-500 hover:text-red-400 hover:bg-red-950/30"
                          onClick={() => handleDelete(rule)}
                          title="Delete"
                          aria-label={`Delete ${rule.pattern}`}
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
          <div className="flex items-center justify-between gap-3 px-4 py-2 text-xs text-gray-400 border-t border-gray-800/50">
            <span>Showing {(page - 1) * 50 + 1}–{Math.min(page * 50, total)} of {total} rules{isFetching ? ' · Updating…' : ''}</span>
            <div className="flex items-center gap-2">
              <button className="btn-secondary" disabled={page <= 1 || isFetching} onClick={() => setPage(page - 1)}>Previous</button>
              <span>Page {page} of {pages}</span>
              <button className="btn-secondary" disabled={page >= pages || isFetching} onClick={() => setPage(page + 1)}>Next</button>
            </div>
          </div>
        </div>
      )}

      {folderEditor !== null && <FolderForm
        folder={folderEditor === 'new' ? null : folderEditor}
        onClose={() => setFolderEditor(null)}
        onSubmit={name => saveFolder.mutate({ name, id: folderEditor === 'new' ? undefined : folderEditor.id })}
        loading={saveFolder.isPending}
        error={saveFolder.error ? getApiError(saveFolder.error) : undefined}
      />}

      {(showForm || editRule) && (
        <RuleFormModal
          rule={editRule}
          exits={exits}
          folders={folders}
          defaultFolder={typeof folder === 'number' ? folder : null}
          onClose={() => { setShowForm(false); setEditRule(null) }}
          onSubmit={data => {
            if ('rules' in data) createBulkMut.mutate(data.rules)
            else if (editRule) updateMut.mutate({ id: editRule.id, data })
            else createMut.mutate(data)
          }}
          loading={createMut.isPending || createBulkMut.isPending || updateMut.isPending}
          error={(createMut.error && getApiError(createMut.error)) || (createBulkMut.error && getApiError(createBulkMut.error)) || (updateMut.error && getApiError(updateMut.error)) || ''}
        />
      )}
    </div>
  )
}

function FolderForm({ folder, onClose, onSubmit, loading, error }: {
  folder: RuleFolder | null
  onClose: () => void
  onSubmit: (name: string) => void
  loading: boolean
  error?: string
}) {
  const [name, setName] = useState(folder?.name ?? '')
  const titleId = useId()
  const nameId = useId()
  const dialog = useRef<HTMLDialogElement>(null)
  const nameInput = useRef<HTMLInputElement>(null)
  useEffect(() => {
    dialog.current?.showModal()
    nameInput.current?.focus()
    nameInput.current?.select()
  }, [])

  return (
    <dialog ref={dialog} className="w-11/12 max-w-sm rounded-2xl border border-gray-700 bg-gray-900 p-0 text-gray-100 shadow-2xl backdrop:bg-black/70"
      aria-labelledby={titleId} onClose={onClose} onCancel={event => { if (loading) event.preventDefault() }}>
      <form onSubmit={event => {
        event.preventDefault()
        const trimmed = name.trim()
        if (trimmed && !loading) onSubmit(trimmed)
      }}>
        <div className="flex items-center justify-between border-b border-gray-800 px-5 py-4">
          <h2 id={titleId} className="font-semibold text-white">{folder ? 'Rename folder' : 'New folder'}</h2>
          <button type="button" className="btn-ghost p-1" aria-label="Close folder form"
            disabled={loading} onClick={() => dialog.current?.close()}><X size={16} /></button>
        </div>
        <div className="space-y-3 p-5">
          <label className="label" htmlFor={nameId}>Folder name</label>
          <input ref={nameInput} id={nameId} className="input" value={name} required
            disabled={loading} onChange={event => setName(event.target.value)} />
          {error && <p role="alert" className="text-sm text-red-300">{error}</p>}
        </div>
        <div className="flex justify-end gap-2 border-t border-gray-800 px-5 py-4">
          <button type="button" className="btn-secondary" disabled={loading} onClick={() => dialog.current?.close()}>Cancel</button>
          <button type="submit" className="btn-primary" disabled={loading || !name.trim()}>
            {loading ? 'Saving…' : folder ? 'Save name' : 'Create folder'}
          </button>
        </div>
      </form>
    </dialog>
  )
}

function RuleFormModal({
  rule, exits, folders, defaultFolder, onClose, onSubmit, loading, error,
}: {
  rule: Rule | null
  exits: Exit[]
  folders: RuleFolder[]
  defaultFolder: number | null
  onClose: () => void
  onSubmit: (data: RuleSubmission) => void
  loading: boolean
  error?: string
}) {
  const [form, setForm] = useState({
    pattern: rule?.pattern ?? '',
    type: (rule?.type ?? 'auto') as RuleFormType,
    exit_id: rule?.exit_id ?? null,
    folder_id: rule ? rule.folder_id ?? null : defaultFolder,
    priority: rule?.priority ?? 100,
    enabled: rule?.enabled ?? true,
  })
  const [localError, setLocalError] = useState('')
  const titleId = useId()
  const folderInputId = useId()
  const dialog = useRef<HTMLDialogElement>(null)
  const patternInput = useRef<HTMLInputElement>(null)
  const patternsInput = useRef<HTMLTextAreaElement>(null)
  useEffect(() => {
    dialog.current?.showModal()
    const input = patternInput.current ?? patternsInput.current
    input?.focus()
  }, [])

  const set = <K extends keyof typeof form>(key: K, value: (typeof form)[K]) => setForm(f => ({ ...f, [key]: value }))
  const patterns = rule ? [form.pattern.trim()].filter(Boolean) : parseRulePatterns(form.pattern)

  const handleSubmit = () => {
    if (patterns.length === 0) {
      setLocalError('At least one pattern is required')
      return
    }
    setLocalError('')

    if (rule) {
      const { pattern, exit_id, folder_id, priority, enabled } = form
      const type = resolveRuleType(pattern, form.type)
      if (pattern === rule.pattern && type === rule.type && exit_id === rule.exit_id &&
        priority === rule.priority && enabled === rule.enabled && folder_id !== (rule.folder_id ?? null)) {
        onSubmit({ folder_id })
      } else {
        onSubmit({ pattern, type, exit_id, folder_id, priority, enabled })
      }
      return
    }

    const rules = patterns.map(pattern => ({
      pattern,
      type: resolveRuleType(pattern, form.type),
      exit_id: form.exit_id,
      folder_id: form.folder_id,
      priority: form.priority,
      enabled: form.enabled,
    }))
    onSubmit(rules.length === 1 ? rules[0] : { rules })
  }

  const displayError = localError || error

  return (
    <dialog ref={dialog} aria-labelledby={titleId} onClose={onClose} onCancel={event => { if (loading) event.preventDefault() }}
      className="w-11/12 max-w-md rounded-2xl border border-gray-700 bg-gray-900 p-0 text-gray-100 shadow-2xl backdrop:bg-black/70">
      <div className="flex max-h-[90dvh] flex-col">
        <div className="flex shrink-0 items-center justify-between px-5 py-4 border-b border-gray-800">
          <h2 id={titleId} className="font-semibold text-white">{rule ? 'Edit Rule' : 'New Rule'}</h2>
          <button className="btn-ghost p-1" aria-label="Close rule form" disabled={loading}
            onClick={() => dialog.current?.close()}><X size={16} /></button>
        </div>
        <div className="space-y-4 overflow-y-auto p-5">
          <div>
            <label className="label">{rule ? 'Pattern *' : 'Patterns *'}</label>
            {rule ? (
              <input
                ref={patternInput}
                aria-label="Pattern"
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
                ref={patternsInput}
                aria-label="Patterns"
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
          <div>
            <label className="label" htmlFor={folderInputId}>Folder</label>
            <select id={folderInputId} className="select" value={form.folder_id ?? ''}
              onChange={event => set('folder_id', event.target.value ? Number(event.target.value) : null)}>
              <option value="">Unfiled</option>
              {form.folder_id != null && !folders.some(item => item.id === form.folder_id) &&
                <option value={form.folder_id}>Current folder</option>}
              {folders.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}
            </select>
          </div>
          <label className="flex items-center gap-2 cursor-pointer select-none">
            <div
              className={`relative w-9 h-5 rounded-full transition-colors ${form.enabled ? 'bg-indigo-600' : 'bg-gray-700'}`}
              onClick={() => set('enabled', !form.enabled)}
            >
              <div className={`absolute top-0.5 w-4 h-4 rounded-full bg-white shadow transition-all ${form.enabled ? 'left-4' : 'left-0.5'}`} />
            </div>
            <span className="text-sm text-gray-300">Enabled</span>
          </label>
          {displayError && (
            <div className="text-red-400 text-sm bg-red-950/40 border border-red-800/50 px-3 py-2.5 rounded-xl">
              {displayError}
            </div>
          )}
        </div>
        <div className="flex shrink-0 justify-end gap-2 px-5 py-4 border-t border-gray-800">
          <button className="btn-secondary" disabled={loading} onClick={() => dialog.current?.close()}>Cancel</button>
          <button
            className="btn-primary"
            disabled={loading || patterns.length === 0}
            onClick={handleSubmit}
          >
            {loading ? 'Saving...' : rule ? 'Update Rule' : patterns.length > 1 ? `Create ${patterns.length} Rules` : 'Create Rule'}
          </button>
        </div>
      </div>
    </dialog>
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
