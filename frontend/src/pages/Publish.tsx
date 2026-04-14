import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  previewConfig, validateConfig, applyConfig, rollbackConfig, getPublishHistory
} from '../api/client'
import { type PublishHistory } from '../api/client'
import { Eye, CheckCircle, Upload, RotateCcw, XCircle, AlertCircle, Clock } from 'lucide-react'

export default function Publish() {
  const qc = useQueryClient()
  const { data: history = [] } = useQuery({ queryKey: ['publish-history'], queryFn: getPublishHistory })
  const [preview, setPreview] = useState<string | null>(null)
  const [validationResult, setValidationResult] = useState<{ valid: boolean; error?: string } | null>(null)
  const [msg, setMsg] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  const invalidate = () => qc.invalidateQueries({ queryKey: ['publish-history'] })

  const show = (type: 'success' | 'error', text: string) => {
    setMsg({ type, text })
    setTimeout(() => setMsg(null), 4000)
  }

  const handlePreview = async () => {
    try {
      const res = await previewConfig()
      setPreview(res.config)
    } catch (e: any) {
      show('error', e?.response?.data?.error ?? e.message)
    }
  }

  const handleValidate = async () => {
    try {
      const res = await validateConfig()
      setValidationResult(res)
    } catch (e: any) {
      setValidationResult({ valid: false, error: e?.response?.data?.error ?? e.message })
    }
  }

  const handleApply = async () => {
    if (!confirm('套用配置並 reload Nginx？')) return
    try {
      await applyConfig()
      show('success', '配置已套用，Nginx 已 reload')
      invalidate()
    } catch (e: any) {
      show('error', e?.response?.data?.error ?? e.message)
      invalidate()
    }
  }

  const handleRollback = async () => {
    if (!confirm('回滾至上一版本配置？')) return
    try {
      await rollbackConfig()
      show('success', '已回滾至上一版本')
      invalidate()
    } catch (e: any) {
      show('error', e?.response?.data?.error ?? e.message)
    }
  }

  return (
    <div className="space-y-5 max-w-5xl">
      <div>
        <h1 className="text-2xl font-bold text-white">發布管理</h1>
        <p className="text-sm text-gray-400 mt-1">將規則編譯為 Nginx 設定並套用</p>
      </div>

      {msg && (
        <div className={`flex items-center gap-2 px-4 py-3 rounded-lg text-sm ${
          msg.type === 'success' ? 'bg-green-900/50 text-green-300' : 'bg-red-900/50 text-red-300'
        }`}>
          {msg.type === 'success' ? <CheckCircle size={16} /> : <XCircle size={16} />}
          {msg.text}
        </div>
      )}

      {/* Action cards */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <button className="card hover:border-indigo-700 transition-colors text-left" onClick={handlePreview}>
          <Eye size={20} className="text-indigo-400 mb-2" />
          <div className="font-medium text-sm text-white">預覽配置</div>
          <div className="text-xs text-gray-400 mt-1">查看即將產生的 Nginx 設定</div>
        </button>
        <button className="card hover:border-blue-700 transition-colors text-left" onClick={handleValidate}>
          <CheckCircle size={20} className="text-blue-400 mb-2" />
          <div className="font-medium text-sm text-white">驗證配置</div>
          <div className="text-xs text-gray-400 mt-1">執行語法與邏輯驗證</div>
        </button>
        <button className="card hover:border-green-700 transition-colors text-left" onClick={handleApply}>
          <Upload size={20} className="text-green-400 mb-2" />
          <div className="font-medium text-sm text-white">套用配置</div>
          <div className="text-xs text-gray-400 mt-1">寫入設定並 reload Nginx</div>
        </button>
        <button className="card hover:border-yellow-700 transition-colors text-left" onClick={handleRollback}>
          <RotateCcw size={20} className="text-yellow-400 mb-2" />
          <div className="font-medium text-sm text-white">回滾</div>
          <div className="text-xs text-gray-400 mt-1">還原至上一版本設定</div>
        </button>
      </div>

      {/* Validation result */}
      {validationResult && (
        <div className={`card flex items-start gap-3 ${
          validationResult.valid ? 'border-green-700' : 'border-red-700'
        }`}>
          {validationResult.valid
            ? <CheckCircle size={18} className="text-green-400 mt-0.5 flex-shrink-0" />
            : <XCircle size={18} className="text-red-400 mt-0.5 flex-shrink-0" />
          }
          <div>
            <div className={`text-sm font-medium ${validationResult.valid ? 'text-green-300' : 'text-red-300'}`}>
              {validationResult.valid ? '配置驗證通過' : '配置驗證失敗'}
            </div>
            {validationResult.error && (
              <pre className="text-xs text-red-400 mt-1 whitespace-pre-wrap">{validationResult.error}</pre>
            )}
          </div>
        </div>
      )}

      {/* Config preview */}
      {preview && (
        <div className="card p-0 overflow-hidden">
          <div className="flex items-center justify-between px-4 py-3 border-b border-gray-800">
            <span className="text-sm font-medium text-gray-300">配置預覽</span>
            <button className="btn-ghost text-xs p-1" onClick={() => setPreview(null)}>關閉</button>
          </div>
          <pre className="p-4 text-xs font-mono text-green-400 bg-black/30 overflow-auto max-h-96 whitespace-pre-wrap">
            {preview}
          </pre>
        </div>
      )}

      {/* History */}
      <div className="card p-0 overflow-hidden">
        <div className="px-4 py-3 border-b border-gray-800">
          <h2 className="text-sm font-semibold text-gray-300">發布紀錄</h2>
        </div>
        {history.length === 0 ? (
          <div className="px-4 py-8 text-center text-gray-500 text-sm">尚無發布紀錄</div>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-xs text-gray-500 border-b border-gray-800">
                <th className="px-4 py-3">版本</th>
                <th className="px-4 py-3">狀態</th>
                <th className="px-4 py-3">說明</th>
                <th className="px-4 py-3">時間</th>
              </tr>
            </thead>
            <tbody>
              {history.map(h => (
                <HistoryRow key={h.id} h={h} />
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}

function HistoryRow({ h }: { h: PublishHistory }) {
  const icon = h.status === 'success'
    ? <CheckCircle size={14} className="text-green-400" />
    : h.status === 'failed'
    ? <XCircle size={14} className="text-red-400" />
    : <RotateCcw size={14} className="text-yellow-400" />

  return (
    <tr className="table-row-hover border-b border-gray-800/50 last:border-0">
      <td className="px-4 py-3 font-mono text-gray-300 text-xs">{h.version}</td>
      <td className="px-4 py-3">
        <div className="flex items-center gap-1.5">{icon}
          <span className={`text-xs ${
            h.status === 'success' ? 'text-green-400' :
            h.status === 'failed' ? 'text-red-400' : 'text-yellow-400'
          }`}>{h.status}</span>
        </div>
      </td>
      <td className="px-4 py-3 text-gray-400 text-xs">{h.message}</td>
      <td className="px-4 py-3 text-gray-500 text-xs">
        {new Date(h.created_at).toLocaleString('zh-TW')}
      </td>
    </tr>
  )
}
