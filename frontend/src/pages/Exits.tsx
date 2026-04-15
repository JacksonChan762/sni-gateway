import { useEffect, useState, type ChangeEvent } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import axios from 'axios'
import {
  getExits, createExit, updateExit, deleteExit,
  startExit, stopExit, restartExit, getExitLogs, getExitConfig
} from '../api/client'
import { type Exit } from '../api/client'
import {
  Plus, Play, Square, RotateCcw, Trash2, Terminal,
  Globe, X, Pencil
} from 'lucide-react'

const wireguardOrder = [
  'VPN_SERVICE_PROVIDER',
  'VPN_TYPE',
  'SERVER_COUNTRIES',
  'SERVER_CITIES',
  'WIREGUARD_PRIVATE_KEY',
  'WIREGUARD_ADDRESSES',
]

const openvpnOrder = [
  'VPN_SERVICE_PROVIDER',
  'VPN_TYPE',
  'SERVER_COUNTRIES',
  'SERVER_CITIES',
  'OPENVPN_USER',
  'OPENVPN_PASSWORD',
]

const envLabels: Record<string, string> = {
  VPN_SERVICE_PROVIDER: '供應商',
  VPN_TYPE: '協定',
  SERVER_COUNTRIES: '國家',
  SERVER_CITIES: '城市',
  WIREGUARD_PRIVATE_KEY: 'WireGuard Private Key',
  WIREGUARD_ADDRESSES: 'WireGuard Addresses',
  OPENVPN_USER: 'OpenVPN User',
  OPENVPN_PASSWORD: 'OpenVPN Password',
}

function parseEnvYamlBlock(input: string): Record<string, string> {
  const result: Record<string, string> = {}

  for (const rawLine of input.split('\n')) {
    const line = rawLine.trim()
    if (!line || line.startsWith('#')) continue
    const idx = line.includes('=') ? line.indexOf('=') : line.indexOf(':')
    if (idx === -1) continue
    const key = line.slice(0, idx).trim()
    let value = line.slice(idx + 1).trim()
    if ((value.startsWith('"') && value.endsWith('"')) || (value.startsWith("'") && value.endsWith("'"))) {
      value = value.slice(1, -1)
    }
    result[key] = value
  }

  return result
}

function stringifyEnvYamlBlock(env: Record<string, string>, protocol: 'wireguard' | 'openvpn') {
  const order = protocol === 'openvpn' ? openvpnOrder : wireguardOrder
  const lines = order.map((key) => `${key}=${env[key] ?? ''}`)
  return `${lines.join('\n')}\n`
}

function convertEnvironmentBlock(input: string, protocol: 'wireguard' | 'openvpn') {
  const env = parseEnvYamlBlock(input)
  env.VPN_TYPE = protocol
  return stringifyEnvYamlBlock(env, protocol)
}

function buildComposeYaml(enabled: boolean, environmentYaml: string) {
  const envLines = environmentYaml
    .trimEnd()
    .split('\n')
    .filter(Boolean)
    .map((line) => `      - ${line.trim()}`)
    .join('\n')

  return `enabled: ${enabled ? 'true' : 'false'}
services:
  gluetun:
    image: qmcgaw/gluetun:latest
    environment:
${envLines}
`
}

function extractEnvironmentBlock(configYaml: string) {
  const lines = configYaml.split('\n')
  const envIndex = lines.findIndex((line) => line.trim() === 'environment:')
  if (envIndex === -1) {
    return stringifyEnvYamlBlock({
      VPN_SERVICE_PROVIDER: 'protonvpn',
      VPN_TYPE: 'wireguard',
      SERVER_COUNTRIES: 'United States',
      SERVER_CITIES: '',
      WIREGUARD_PRIVATE_KEY: '',
      WIREGUARD_ADDRESSES: '',
    }, 'wireguard')
  }

  const extracted: string[] = []
  for (let i = envIndex + 1; i < lines.length; i += 1) {
    const line = lines[i]
    if (line.startsWith('      - ')) {
      extracted.push(line.slice(8))
      continue
    }
    if (line.startsWith('      ')) {
      const trimmed = line.slice(6)
      extracted.push(trimmed.replace(': ', '='))
      continue
    }
    break
  }

  return extracted.length > 0 ? `${extracted.join('\n')}\n` : ''
}

function getErrorMessage(error: unknown) {
  if (axios.isAxiosError(error)) {
    const apiMessage = error.response?.data?.error
    if (typeof apiMessage === 'string' && apiMessage.trim()) {
      return apiMessage
    }
  }
  if (error instanceof Error) {
    return error.message
  }
  return 'unknown error'
}

function validateExitForm(name: string, protocol: 'wireguard' | 'openvpn', environmentYaml: string) {
  if (!name.trim()) {
    return 'name is required'
  }

  const env = parseEnvYamlBlock(environmentYaml)
  if (!env.VPN_SERVICE_PROVIDER) {
    return 'VPN_SERVICE_PROVIDER is required'
  }
  if (!env.SERVER_COUNTRIES && !env.SERVER_CITIES) {
    return 'SERVER_COUNTRIES or SERVER_CITIES is required'
  }
  if (protocol === 'wireguard' && !env.WIREGUARD_PRIVATE_KEY) {
    return 'WIREGUARD_PRIVATE_KEY is required'
  }
  if (protocol === 'openvpn' && (!env.OPENVPN_USER || !env.OPENVPN_PASSWORD)) {
    return 'OPENVPN_USER and OPENVPN_PASSWORD are required'
  }

  return ''
}

function envFieldOrder(protocol: 'wireguard' | 'openvpn') {
  return protocol === 'openvpn' ? openvpnOrder : wireguardOrder
}

export default function Exits() {
  const qc = useQueryClient()
  const { data: exits = [], isLoading } = useQuery({ queryKey: ['exits'], queryFn: getExits })
  const [showForm, setShowForm] = useState(false)
  const [logModal, setLogModal] = useState<{ exit: Exit; logs: string } | null>(null)
  const [editExit, setEditExit] = useState<Exit | null>(null)

  const invalidate = async () => {
    await qc.invalidateQueries({ queryKey: ['exits'] })
    await qc.refetchQueries({ queryKey: ['exits'] })
  }

  const createMut = useMutation({
    mutationFn: createExit,
    onSuccess: async () => {
      await invalidate()
      setShowForm(false)
    },
  })
  const updateMut = useMutation({
    mutationFn: ({ id, data }: { id: number; data: { config_yaml: string; enabled: boolean } }) => updateExit(id, data),
    onSuccess: async () => {
      await invalidate()
      setEditExit(null)
    },
  })
  const deleteMut = useMutation({
    mutationFn: deleteExit,
    onSuccess: async (_, id) => {
      qc.setQueryData<Exit[]>(['exits'], prev => (prev ?? []).filter(exit => exit.id !== id))
      await invalidate()
    },
  })
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

      {(createMut.error || updateMut.error || deleteMut.error || startMut.error || stopMut.error || restartMut.error) && (
        <div className="card border-red-700 text-red-300 text-sm">
          {getErrorMessage(createMut.error) ||
            getErrorMessage(updateMut.error) ||
            getErrorMessage(deleteMut.error) ||
            getErrorMessage(startMut.error) ||
            getErrorMessage(stopMut.error) ||
            getErrorMessage(restartMut.error)}
        </div>
      )}

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
              onEdit={() => setEditExit(exit)}
              onDelete={() => {
                if (confirm(`確定刪除出口 "${exit.name}"？`)) deleteMut.mutate(exit.id)
              }}
              onLogs={() => handleLogs(exit)}
            />
          ))}
        </div>
      )}

      {(showForm || editExit) && (
        <ExitFormModal
          exit={editExit}
          onClose={() => {
            setShowForm(false)
            setEditExit(null)
          }}
          onSubmit={(data) => {
            if (editExit) {
              updateMut.mutate({ id: editExit.id, data })
            } else {
              createMut.mutate(data)
            }
          }}
          loading={createMut.isPending || updateMut.isPending}
          error={getErrorMessage(createMut.error) || getErrorMessage(updateMut.error)}
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
  exit, onStart, onStop, onRestart, onEdit, onDelete, onLogs
}: {
  exit: Exit
  onStart: () => void
  onStop: () => void
  onRestart: () => void
  onEdit: () => void
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
          <button className="btn-ghost text-xs" onClick={onEdit}><Pencil size={13} /> 編輯</button>
          <button className="btn-ghost text-xs" onClick={onLogs}><Terminal size={13} /> 日誌</button>
          <button className="btn-danger text-xs" onClick={onDelete}><Trash2 size={13} /></button>
        </div>
      </div>
    </div>
  )
}

function ExitFormModal({
  exit, onClose, onSubmit, loading, error
}: {
  exit: Exit | null
  onClose: () => void
  onSubmit: (data: any) => void
  loading: boolean
  error?: string
}) {
  const { data: configData, isLoading: configLoading } = useQuery({
    queryKey: ['exit-config', exit?.id],
    queryFn: () => getExitConfig(exit!.id),
    enabled: Boolean(exit),
  })
  const [form, setForm] = useState({
    name: exit?.name ?? '',
    enabled: exit?.enabled ?? true,
    protocol: 'wireguard' as 'wireguard' | 'openvpn',
    environment_yaml: stringifyEnvYamlBlock({
      VPN_SERVICE_PROVIDER: 'protonvpn',
      VPN_TYPE: 'wireguard',
      SERVER_COUNTRIES: 'United States',
      SERVER_CITIES: '',
      WIREGUARD_PRIVATE_KEY: '',
      WIREGUARD_ADDRESSES: '',
    }, 'wireguard'),
  })
  const [localError, setLocalError] = useState('')

  const env = parseEnvYamlBlock(form.environment_yaml)
  const fields = envFieldOrder(form.protocol)

  const updateEnvField = (key: string, value: string) => {
    const nextEnv = parseEnvYamlBlock(form.environment_yaml)
    nextEnv[key] = value
    if (!nextEnv.VPN_TYPE) {
      nextEnv.VPN_TYPE = form.protocol
    }
    setForm((f) => ({
      ...f,
      environment_yaml: stringifyEnvYamlBlock(nextEnv, f.protocol),
    }))
  }

  const handleImportFile = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    if (!file) return

    const content = await file.text()
    const importedEnv = parseEnvYamlBlock(content)
    const protocol = importedEnv.VPN_TYPE === 'openvpn' ? 'openvpn' : 'wireguard'

    setForm((f) => ({
      ...f,
      protocol,
      environment_yaml: stringifyEnvYamlBlock(importedEnv, protocol),
    }))
    setLocalError('')
    event.target.value = ''
  }

  useEffect(() => {
    if (!exit) {
      return
    }
    if (configData?.config_yaml) {
      const environmentYaml = extractEnvironmentBlock(configData.config_yaml)
      const env = parseEnvYamlBlock(environmentYaml)
      const protocol = env.VPN_TYPE === 'openvpn' ? 'openvpn' : 'wireguard'
      setForm({
        name: exit.name,
        enabled: exit.enabled,
        protocol,
        environment_yaml: stringifyEnvYamlBlock(env, protocol),
      })
      setLocalError('')
    }
  }, [configData, exit])

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4">
      <div className="bg-gray-900 border border-gray-700 rounded-xl w-full max-w-lg">
        <div className="flex items-center justify-between px-5 py-4 border-b border-gray-800">
          <h3 className="font-semibold text-white">{exit ? '編輯出口' : '新增出口'}</h3>
          <button className="btn-ghost p-1" onClick={onClose}><X size={16} /></button>
        </div>
        <div className="p-5 space-y-4">
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="label">出口名稱 *</label>
              <input
                className="input"
                placeholder="us"
                value={form.name}
                disabled={Boolean(exit)}
                onChange={e => setForm(f => ({ ...f, name: e.target.value }))}
              />
            </div>
            <div>
              <label className="label">模式</label>
              <input className="input text-gray-400" value="只可編輯 environment" disabled />
            </div>
          </div>

          <div>
            <label className="label">VPN 協定</label>
            <div className="flex gap-2">
              <button
                type="button"
                className={form.protocol === 'wireguard' ? 'btn-primary' : 'btn-secondary'}
                onClick={() => setForm((f) => ({
                  ...f,
                  protocol: 'wireguard',
                  environment_yaml: convertEnvironmentBlock(f.environment_yaml, 'wireguard'),
                }))}
              >
                WireGuard
              </button>
              <button
                type="button"
                className={form.protocol === 'openvpn' ? 'btn-primary' : 'btn-secondary'}
                onClick={() => setForm((f) => ({
                  ...f,
                  protocol: 'openvpn',
                  environment_yaml: convertEnvironmentBlock(f.environment_yaml, 'openvpn'),
                }))}
              >
                OpenVPN 一键切换
              </button>
            </div>
          </div>

          <div>
            <label className="label">導入配置</label>
            <input
              className="input"
              type="file"
              accept=".env,.txt"
              onChange={handleImportFile}
            />
            <p className="text-xs text-gray-500 mt-1">
              支援 `KEY=VALUE` 格式檔案導入。
            </p>
          </div>

          <div className="flex items-center gap-2">
            <input
              type="checkbox"
              id="exit-enabled"
              checked={form.enabled}
              onChange={e => setForm(f => ({ ...f, enabled: e.target.checked }))}
            />
            <label htmlFor="exit-enabled" className="text-sm text-gray-300">啟用出口</label>
          </div>

          <div>
            <label className="label">environment</label>
            <div className="space-y-3">
              {fields.map((key) => (
                <div key={key}>
                  <label className="label">{envLabels[key] ?? key}</label>
                  <input
                    className="input font-mono text-xs"
                    type={key.includes('PASSWORD') ? 'password' : 'text'}
                    value={env[key] ?? ''}
                    onChange={(e) => updateEnvField(key, e.target.value)}
                  />
                  <p className="text-xs text-gray-500 mt-1">{key}</p>
                </div>
              ))}
            </div>
            <p className="text-xs text-gray-500 mt-1">
              只會修改 `services.gluetun.environment`，外層 compose 結構固定。
            </p>
            <p className="text-xs text-gray-500 mt-1">
              `SERVER_COUNTRIES` 和 `SERVER_CITIES` 二選一即可；`WIREGUARD_ADDRESSES` 可留空。
            </p>
            {configLoading && <p className="text-xs text-gray-500 mt-2">載入目前配置中...</p>}
          </div>

          <div>
            <label className="label">Compose 預覽</label>
            <pre className="input min-h-[180px] font-mono text-xs text-gray-400 whitespace-pre-wrap">
              {buildComposeYaml(form.enabled, form.environment_yaml)}
            </pre>
          </div>

          {error && <div className="text-red-400 text-sm">{error}</div>}
          {localError && <div className="text-red-400 text-sm">{localError}</div>}
        </div>
        <div className="flex justify-end gap-2 px-5 py-4 border-t border-gray-800">
          <button className="btn-secondary" onClick={onClose}>取消</button>
          <button
            className="btn-primary"
            disabled={loading || !form.name}
            onClick={() => {
              const validationError = validateExitForm(form.name, form.protocol, form.environment_yaml)
              if (validationError) {
                setLocalError(validationError)
                return
              }
              setLocalError('')
              onSubmit({
                name: form.name,
                enabled: form.enabled,
                config_yaml: buildComposeYaml(form.enabled, form.environment_yaml),
              })
            }}
          >
            {loading ? '儲存中...' : exit ? '更新出口' : '建立出口'}
          </button>
        </div>
      </div>
    </div>
  )
}
