import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  previewConfig, validateConfig, applyConfig, rollbackConfig,
  getPublishHistory, getApiError, testRoute, type RouteMatch,
} from '../api/client'
import { type PublishHistory } from '../api/client'
import {
  Eye, CheckCircle, Upload, RotateCcw, XCircle, Clock,
  ChevronDown, ChevronUp, History,
} from 'lucide-react'
import { useToast } from '../components/Toast'
import { useConfirm } from '../components/ConfirmModal'

export default function Publish() {
  const qc = useQueryClient()
  const toast = useToast()
  const { confirm, modal: confirmModal } = useConfirm()
  const { data: history = [], isLoading: histLoading } = useQuery({
    queryKey: ['publish-history'],
    queryFn: getPublishHistory,
  })
  const [host, setHost] = useState('')
  const [match, setMatch] = useState<RouteMatch | null>(null)
  const [matchLoading, setMatchLoading] = useState(false)
  const [matchError, setMatchError] = useState('')
  const [preview, setPreview] = useState<string | null>(null)
  const [validationResult, setValidationResult] = useState<{ valid: boolean; error?: string } | null>(null)
  const [previewLoading, setPreviewLoading] = useState(false)
  const [validateLoading, setValidateLoading] = useState(false)
  const [applyLoading, setApplyLoading] = useState(false)
  const [rollbackLoading, setRollbackLoading] = useState(false)

  const invalidateHistory = () => qc.invalidateQueries({ queryKey: ['publish-history'] })

  const handlePreview = async () => {
    setPreviewLoading(true)
    try {
      const res = await previewConfig()
      setPreview(res.config)
    } catch (err) {
      toast.error(getApiError(err))
    } finally {
      setPreviewLoading(false)
    }
  }

  const handleValidate = async () => {
    setValidateLoading(true)
    try {
      const res = await validateConfig()
      setValidationResult(res)
      if (res.valid) toast.success('Configuration is valid')
      else toast.error('Validation failed')
    } catch (err) {
      const msg = getApiError(err)
      setValidationResult({ valid: false, error: msg })
      toast.error(msg)
    } finally {
      setValidateLoading(false)
    }
  }

  const handleApply = async () => {
    const ok = await confirm({
      title: 'Apply Configuration',
      message: 'This will write the generated config to nginx and reload it. Active traffic may briefly be affected.',
      confirmText: 'Apply & Reload',
    })
    if (!ok) return
    setApplyLoading(true)
    try {
      await applyConfig()
      toast.success('Configuration applied — nginx reloaded')
      invalidateHistory()
      qc.invalidateQueries({ queryKey: ['stats'] })
    } catch (err) {
      toast.error(getApiError(err))
      invalidateHistory()
    } finally {
      setApplyLoading(false)
    }
  }

  const handleRollback = async () => {
    const ok = await confirm({
      title: 'Rollback Configuration',
      message: 'Restore the previous nginx configuration? Current rules will no longer be active.',
      confirmText: 'Rollback',
      dangerous: true,
    })
    if (!ok) return
    setRollbackLoading(true)
    try {
      await rollbackConfig()
      toast.success('Rolled back to previous configuration')
      invalidateHistory()
    } catch (err) {
      toast.error(getApiError(err))
    } finally {
      setRollbackLoading(false)
    }
  }

  const actionCards = [
    {
      label: 'Preview Config',
      desc: 'View generated nginx config before applying',
      icon: Eye,
      color: 'text-indigo-400',
      border: 'hover:border-indigo-700/60',
      bg: 'hover:bg-indigo-950/20',
      onClick: handlePreview,
      loading: previewLoading,
    },
    {
      label: 'Validate',
      desc: 'Check for syntax and logic errors',
      icon: CheckCircle,
      color: 'text-blue-400',
      border: 'hover:border-blue-700/60',
      bg: 'hover:bg-blue-950/20',
      onClick: handleValidate,
      loading: validateLoading,
    },
    {
      label: 'Apply Config',
      desc: 'Write config and reload nginx',
      icon: Upload,
      color: 'text-green-400',
      border: 'hover:border-green-700/60',
      bg: 'hover:bg-green-950/20',
      onClick: handleApply,
      loading: applyLoading,
    },
    {
      label: 'Rollback',
      desc: 'Restore previous configuration',
      icon: RotateCcw,
      color: 'text-yellow-400',
      border: 'hover:border-yellow-700/60',
      bg: 'hover:bg-yellow-950/20',
      onClick: handleRollback,
      loading: rollbackLoading,
    },
  ]

  return (
    <div className="space-y-6">
      {confirmModal}

      <div>
        <h1 className="page-title">Publish</h1>
        <p className="page-subtitle">Compile rules into nginx config and apply</p>
      </div>

      {/* Action cards */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        {actionCards.map(({ label, desc, icon: Icon, color, border, bg, onClick, loading }) => (
          <button
            key={label}
            className={`card text-left transition-all ${border} ${bg} disabled:opacity-60`}
            onClick={onClick}
            disabled={loading}
          >
            <div className={`w-9 h-9 rounded-xl flex items-center justify-center mb-3 ${
              loading ? 'bg-gray-800' : 'bg-gray-800/60'
            }`}>
              {loading ? (
                <svg className="animate-spin h-4 w-4 text-gray-400" viewBox="0 0 24 24" fill="none">
                  <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                  <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                </svg>
              ) : (
                <Icon size={18} className={color} />
              )}
            </div>
            <div className="font-medium text-sm text-white mb-0.5">{label}</div>
            <div className="text-xs text-gray-500 leading-relaxed">{desc}</div>
          </button>
        ))}
      </div>

      <form className="card space-y-3" onSubmit={async e => {
        e.preventDefault()
        setMatchLoading(true); setMatch(null); setMatchError('')
        try { setMatch(await testRoute(host)) }
        catch (error) { setMatchError(getApiError(error)) }
        finally { setMatchLoading(false) }
      }}>
        <label htmlFor="route-host" className="block font-medium text-sm">Test domain routing</label>
        <p className="text-xs text-gray-400">Preview saved rules, including unpublished changes. Higher priority wins; equal priorities use the lowest rule ID.</p>
        <div className="flex gap-2">
          <input id="route-host" className="input flex-1" placeholder="example.com" value={host} disabled={matchLoading} onChange={e => { setHost(e.target.value); setMatch(null); setMatchError('') }} required />
          <button className="btn-primary" disabled={matchLoading}>{matchLoading ? 'Checking…' : 'Test Match'}</button>
        </div>
        {matchError && <p role="alert" className="text-sm text-red-400">{matchError}</p>}
        {match && <p role="status" className="text-sm text-gray-300">
          {match.matched ? `${match.host} → ${match.exit?.name} (${match.exit?.status}); ${match.rule?.type} ${match.rule?.pattern}, priority ${match.rule?.priority}` : `${match.host}: no matching route`}
        </p>}
      </form>

      {/* Workflow hint */}
      <div className="flex items-center gap-2 text-xs text-gray-600">
        <span className="px-2 py-1 rounded bg-gray-800 text-gray-500">Preview</span>
        <span>→</span>
        <span className="px-2 py-1 rounded bg-gray-800 text-gray-500">Validate</span>
        <span>→</span>
        <span className="px-2 py-1 rounded bg-gray-800 text-gray-500">Apply</span>
        <span className="ml-2 text-gray-700">recommended workflow</span>
      </div>

      {/* Validation result */}
      {validationResult && (
        <div className={`card flex items-start gap-3 ${
          validationResult.valid
            ? 'border-green-700/40 bg-green-950/20'
            : 'border-red-700/40 bg-red-950/20'
        }`}>
          {validationResult.valid ? (
            <CheckCircle size={18} className="text-green-400 mt-0.5 flex-shrink-0" />
          ) : (
            <XCircle size={18} className="text-red-400 mt-0.5 flex-shrink-0" />
          )}
          <div>
            <div className={`text-sm font-medium ${validationResult.valid ? 'text-green-300' : 'text-red-300'}`}>
              {validationResult.valid ? 'Configuration is valid' : 'Validation failed'}
            </div>
            {validationResult.error && (
              <pre className="text-xs text-red-400 mt-1.5 whitespace-pre-wrap font-mono bg-red-950/30 px-2 py-1.5 rounded-lg">
                {validationResult.error}
              </pre>
            )}
          </div>
          <button
            className="ml-auto text-gray-600 hover:text-gray-400 transition-colors flex-shrink-0"
            onClick={() => setValidationResult(null)}
            title="Dismiss"
          >
            <XCircle size={14} />
          </button>
        </div>
      )}

      {/* Config preview */}
      {preview && (
        <div className="card p-0 overflow-hidden">
          <div className="flex items-center justify-between px-5 py-3 border-b border-gray-800">
            <span className="text-sm font-medium text-gray-300 flex items-center gap-2">
              <Eye size={14} className="text-indigo-400" />
              Generated Config Preview
            </span>
            <button
              className="btn-ghost text-xs p-1.5"
              onClick={() => setPreview(null)}
            >
              <XCircle size={14} />
            </button>
          </div>
          <pre className="p-5 text-xs font-mono text-green-300 bg-black/40 overflow-auto max-h-96 whitespace-pre-wrap leading-relaxed">
            {preview}
          </pre>
        </div>
      )}

      {/* History */}
      <div className="card p-0 overflow-hidden">
        <div className="flex items-center gap-2 px-5 py-4 border-b border-gray-800">
          <History size={15} className="text-gray-500" />
          <h2 className="text-sm font-semibold text-gray-300">Publish History</h2>
          {history.length > 0 && (
            <span className="badge-gray ml-auto">{history.length}</span>
          )}
        </div>
        {histLoading ? (
          <div className="px-5 py-8 text-center text-gray-600 text-sm">Loading history...</div>
        ) : history.length === 0 ? (
          <div className="px-5 py-10 text-center">
            <Clock size={24} className="mx-auto text-gray-700 mb-2" />
            <p className="text-gray-500 text-sm">No publish history yet</p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-gray-800/80">
                  <th className="px-5 py-3 text-left text-xs font-medium text-gray-500">Version</th>
                  <th className="px-5 py-3 text-left text-xs font-medium text-gray-500">Status</th>
                  <th className="px-5 py-3 text-left text-xs font-medium text-gray-500">Message</th>
                  <th className="px-5 py-3 text-left text-xs font-medium text-gray-500">Time</th>
                </tr>
              </thead>
              <tbody>
                {history.map(h => (
                  <HistoryRow key={h.id} h={h} />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}

function HistoryRow({ h }: { h: PublishHistory }) {
  const isSuccess = h.status === 'success'
  const isFailed = h.status === 'failed'
  const isRollback = h.status === 'rollback'

  return (
    <tr className="border-b border-gray-800/40 last:border-0 table-row-hover">
      <td className="px-5 py-3 font-mono text-gray-400 text-xs">{h.version}</td>
      <td className="px-5 py-3">
        <div className="flex items-center gap-1.5">
          {isSuccess && <CheckCircle size={13} className="text-green-400" />}
          {isFailed && <XCircle size={13} className="text-red-400" />}
          {isRollback && <RotateCcw size={13} className="text-yellow-400" />}
          {!isSuccess && !isFailed && !isRollback && <Clock size={13} className="text-gray-500" />}
          <span className={`text-xs font-medium ${
            isSuccess ? 'text-green-400' : isFailed ? 'text-red-400' : isRollback ? 'text-yellow-400' : 'text-gray-400'
          }`}>
            {h.status}
          </span>
        </div>
      </td>
      <td className="px-5 py-3 text-gray-500 text-xs max-w-xs truncate">{h.message}</td>
      <td className="px-5 py-3 text-gray-600 text-xs whitespace-nowrap">
        {new Date(h.created_at).toLocaleString()}
      </td>
    </tr>
  )
}
