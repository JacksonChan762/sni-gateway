import { useEffect, useState, type ChangeEvent } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  getExits, createExit, updateExit, deleteExit,
  startExit, stopExit, restartExit, getExitLogs, getExitConfig, getApiError,
} from '../api/client'
import { type Exit } from '../api/client'
import {
  Plus, Play, Square, RotateCcw, Trash2, Terminal,
  Globe, X, Pencil, Wifi, Shield, Copy,
} from 'lucide-react'
import { useToast } from '../components/Toast'
import { useConfirm } from '../components/ConfirmModal'

// ─── VPN config helpers ───────────────────────────────────────────────────────

const wireguardOrder = [
  'VPN_SERVICE_PROVIDER', 'VPN_TYPE', 'SERVER_COUNTRIES', 'SERVER_CITIES',
  'WIREGUARD_PRIVATE_KEY', 'WIREGUARD_ADDRESSES',
]
const customWireguardOrder = [
  'VPN_SERVICE_PROVIDER', 'VPN_TYPE',
  'WIREGUARD_ENDPOINT_IP', 'WIREGUARD_ENDPOINT_PORT',
  'WIREGUARD_PUBLIC_KEY', 'WIREGUARD_PRIVATE_KEY',
  'WIREGUARD_PRESHARED_KEY', 'WIREGUARD_ADDRESSES',
  'WIREGUARD_DNS', 'WIREGUARD_ALLOWED_IPS', 'WIREGUARD_MTU',
  'WIREGUARD_PERSISTENT_KEEPALIVE_INTERVAL',
]
const openvpnOrder = [
  'VPN_SERVICE_PROVIDER', 'VPN_TYPE', 'SERVER_COUNTRIES', 'SERVER_CITIES',
  'OPENVPN_USER', 'OPENVPN_PASSWORD',
]
const envLabels: Record<string, string> = {
  VPN_SERVICE_PROVIDER: 'VPN Provider',
  VPN_TYPE: 'Protocol',
  SERVER_COUNTRIES: 'Country',
  SERVER_CITIES: 'City',
  WIREGUARD_ENDPOINT_IP: 'WireGuard Endpoint IP',
  WIREGUARD_ENDPOINT_PORT: 'WireGuard Endpoint Port',
  WIREGUARD_PUBLIC_KEY: 'WireGuard Server Public Key',
  WIREGUARD_PRIVATE_KEY: 'WireGuard Private Key',
  WIREGUARD_PRESHARED_KEY: 'WireGuard Pre-shared Key',
  WIREGUARD_ADDRESSES: 'WireGuard Addresses',
  WIREGUARD_DNS: 'WireGuard DNS',
  WIREGUARD_ALLOWED_IPS: 'WireGuard Allowed IPs',
  WIREGUARD_MTU: 'WireGuard MTU',
  WIREGUARD_PERSISTENT_KEEPALIVE_INTERVAL: 'WireGuard Keepalive',
  OPENVPN_USER: 'OpenVPN Username',
  OPENVPN_PASSWORD: 'OpenVPN Password',
}

function isCustomProvider(env: Record<string, string>) {
  return env.VPN_SERVICE_PROVIDER?.trim().toLowerCase() === 'custom'
}

function getOrderedEnvKeys(env: Record<string, string>, protocol: 'wireguard' | 'openvpn') {
  if (protocol === 'openvpn') return openvpnOrder
  return isCustomProvider(env) ? customWireguardOrder : wireguardOrder
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
    if ((value.startsWith('"') && value.endsWith('"')) ||
        (value.startsWith("'") && value.endsWith("'"))) {
      value = value.slice(1, -1)
    }
    result[key] = value
  }
  return result
}

function stringifyEnvYamlBlock(
  env: Record<string, string>,
  protocol: 'wireguard' | 'openvpn'
) {
  const order = getOrderedEnvKeys(env, protocol)
  const orderedKeys = new Set(order)
  const extraKeys = Object.keys(env).filter(key => !orderedKeys.has(key))
  return [...order, ...extraKeys].map(key => `${key}=${env[key] ?? ''}`).join('\n') + '\n'
}

function convertEnvironmentBlock(
  input: string,
  protocol: 'wireguard' | 'openvpn'
) {
  const env = parseEnvYamlBlock(input)
  env.VPN_TYPE = protocol
  return stringifyEnvYamlBlock(env, protocol)
}

function applyCustomWireguardDefaults(env: Record<string, string>) {
  const next = { ...env }
  next.VPN_SERVICE_PROVIDER = 'custom'
  next.VPN_TYPE = 'wireguard'
  for (const key of customWireguardOrder) {
    if (!(key in next)) next[key] = ''
  }
  return next
}

function buildComposeYaml(enabled: boolean, environmentYaml: string) {
  const envLines = environmentYaml
    .trimEnd()
    .split('\n')
    .filter(Boolean)
    .map(line => `      - ${line.trim()}`)
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
  const envIndex = lines.findIndex(l => l.trim() === 'environment:')
  if (envIndex === -1) {
    return stringifyEnvYamlBlock({
      VPN_SERVICE_PROVIDER: 'protonvpn', VPN_TYPE: 'wireguard',
      SERVER_COUNTRIES: 'United States', SERVER_CITIES: '',
      WIREGUARD_PRIVATE_KEY: '', WIREGUARD_ADDRESSES: '',
    }, 'wireguard')
  }
  const envIndent = lines[envIndex].search(/\S/)
  const extracted: string[] = []
  for (let i = envIndex + 1; i < lines.length; i++) {
    const line = lines[i]
    if (!line.trim()) continue
    const indent = line.search(/\S/)
    if (indent <= envIndent) break
    let entry = line.trim()
    if (entry.startsWith('- ')) entry = entry.slice(2).trim()
    if (!entry) continue
    if (!entry.includes('=')) entry = entry.replace(/:\s*/, '=')
    extracted.push(entry)
  }
  return extracted.length > 0 ? extracted.join('\n') + '\n' : ''
}

function parseWireguardConfig(input: string) {
  const env: Record<string, string> = {
    VPN_SERVICE_PROVIDER: 'custom',
    VPN_TYPE: 'wireguard',
  }
  let section = ''

  for (const rawLine of input.split('\n')) {
    const line = rawLine.replace(/[;#].*$/, '').trim()
    if (!line) continue
    const sectionMatch = line.match(/^\[(.+)]$/)
    if (sectionMatch) {
      section = sectionMatch[1].trim().toLowerCase()
      continue
    }

    const [rawKey, ...rawValue] = line.split('=')
    if (!rawKey || rawValue.length === 0) continue
    const key = rawKey.trim().toLowerCase()
    const value = rawValue.join('=').trim()
    if (!value) continue

    if (section === 'interface') {
      if (key === 'privatekey') env.WIREGUARD_PRIVATE_KEY = value
      if (key === 'address') env.WIREGUARD_ADDRESSES = value
      if (key === 'dns') env.WIREGUARD_DNS = value
      if (key === 'mtu') env.WIREGUARD_MTU = value
      continue
    }

    if (section === 'peer') {
      if (key === 'publickey') env.WIREGUARD_PUBLIC_KEY = value
      if (key === 'presharedkey') env.WIREGUARD_PRESHARED_KEY = value
      if (key === 'allowedips') env.WIREGUARD_ALLOWED_IPS = value
      if (key === 'persistentkeepalive') {
        env.WIREGUARD_PERSISTENT_KEEPALIVE_INTERVAL = /^\d+$/.test(value) ? `${value}s` : value
      }
      if (key === 'endpoint') {
        const endpoint = parseWireguardEndpoint(value)
        if (endpoint.host) env.WIREGUARD_ENDPOINT_IP = endpoint.host
        if (endpoint.port) env.WIREGUARD_ENDPOINT_PORT = endpoint.port
      }
    }
  }

  return applyCustomWireguardDefaults(env)
}

function parseWireguardEndpoint(value: string) {
  const trimmed = value.trim()
  const ipv6 = trimmed.match(/^\[([^\]]+)]:(\d+)$/)
  if (ipv6) return { host: ipv6[1], port: ipv6[2] }

  const idx = trimmed.lastIndexOf(':')
  if (idx === -1) return { host: trimmed, port: '' }
  return {
    host: trimmed.slice(0, idx),
    port: trimmed.slice(idx + 1),
  }
}

function validateExitForm(
  name: string,
  protocol: 'wireguard' | 'openvpn',
  environmentYaml: string
) {
  if (!name.trim()) return 'Name is required'
  const env = parseEnvYamlBlock(environmentYaml)
  if (!env.VPN_SERVICE_PROVIDER) return 'VPN_SERVICE_PROVIDER is required'
  const isCustom = isCustomProvider(env)
  if (protocol === 'wireguard' && isCustom) {
    if (!env.WIREGUARD_ENDPOINT_IP) return 'WIREGUARD_ENDPOINT_IP is required for custom WireGuard exits'
    if (!env.WIREGUARD_ENDPOINT_PORT) return 'WIREGUARD_ENDPOINT_PORT is required for custom WireGuard exits'
    if (!env.WIREGUARD_PUBLIC_KEY) return 'WIREGUARD_PUBLIC_KEY is required for custom WireGuard exits'
    if (!env.WIREGUARD_PRIVATE_KEY) return 'WIREGUARD_PRIVATE_KEY is required for custom WireGuard exits'
    if (!env.WIREGUARD_ADDRESSES) return 'WIREGUARD_ADDRESSES is required for custom WireGuard exits'
  }
  if (protocol === 'wireguard' && !env.WIREGUARD_PRIVATE_KEY) return 'WIREGUARD_PRIVATE_KEY is required'
  if (!isCustom && !env.SERVER_COUNTRIES && !env.SERVER_CITIES)
    return 'SERVER_COUNTRIES or SERVER_CITIES is required'
  if (protocol === 'openvpn' && (!env.OPENVPN_USER || !env.OPENVPN_PASSWORD))
    return 'OPENVPN_USER and OPENVPN_PASSWORD are required'
  return ''
}

function normalizeEnvKey(key: string) {
  return key.trim().toUpperCase().replace(/[^A-Z0-9_]/g, '_')
}

function nextCustomEnvKey(env: Record<string, string>) {
  let key = 'CUSTOM_KEY'
  let i = 2
  while (Object.prototype.hasOwnProperty.call(env, key)) {
    key = `CUSTOM_KEY_${i}`
    i += 1
  }
  return key
}

// ─── Status helpers ──────────────────────────────────────────────────────────

function StatusDot({ status }: { status: string }) {
  if (status === 'running') return <span className="status-dot-running" />
  if (status === 'unhealthy') return <span className="status-dot-unhealthy" />
  return <span className="status-dot-stopped" />
}

function StatusBadge({ status }: { status: string }) {
  if (status === 'running') return <span className="badge-green">Running</span>
  if (status === 'stopped') return <span className="badge-gray">Stopped</span>
  if (status === 'unhealthy') return <span className="badge-yellow">Unhealthy</span>
  return <span className="badge-gray">{status}</span>
}

// ─── Main component ───────────────────────────────────────────────────────────

export default function Exits() {
  const qc = useQueryClient()
  const toast = useToast()
  const { confirm, modal: confirmModal } = useConfirm()
  const { data: exits = [], isLoading } = useQuery({ queryKey: ['exits'], queryFn: getExits })
  const [showForm, setShowForm] = useState(false)
  const [logModal, setLogModal] = useState<{ exit: Exit; logs: string } | null>(null)
  const [editExit, setEditExit] = useState<Exit | null>(null)

  const invalidate = async () => {
    await qc.invalidateQueries({ queryKey: ['exits'] })
  }

  const createMut = useMutation({
    mutationFn: createExit,
    onSuccess: async () => { await invalidate(); setShowForm(false); toast.success('Exit created successfully') },
    onError: err => toast.error(getApiError(err)),
  })
  const updateMut = useMutation({
    mutationFn: ({ id, data }: { id: number; data: { name: string; config_yaml: string; enabled: boolean } }) =>
      updateExit(id, data),
    onSuccess: async (_, vars) => {
      qc.removeQueries({ queryKey: ['exit-config', vars.id] })
      await invalidate()
      setEditExit(null)
      toast.success('Exit updated')
    },
    onError: err => toast.error(getApiError(err)),
  })
  const deleteMut = useMutation({
    mutationFn: deleteExit,
    onSuccess: async (_, id) => {
      qc.setQueryData<Exit[]>(['exits'], prev => (prev ?? []).filter(e => e.id !== id))
      await invalidate()
      toast.success('Exit deleted')
    },
    onError: err => toast.error(getApiError(err)),
  })
  const startMut = useMutation({
    mutationFn: startExit,
    onSuccess: async () => { await invalidate(); toast.success('Exit started') },
    onError: err => toast.error(getApiError(err)),
  })
  const stopMut = useMutation({
    mutationFn: stopExit,
    onSuccess: async () => { await invalidate(); toast.success('Exit stopped') },
    onError: err => toast.error(getApiError(err)),
  })
  const restartMut = useMutation({
    mutationFn: restartExit,
    onSuccess: async () => { await invalidate(); toast.success('Exit restarted') },
    onError: err => toast.error(getApiError(err)),
  })

  const handleLogs = async (exit: Exit) => {
    try {
      const res = await getExitLogs(exit.id)
      setLogModal({ exit, logs: res.logs })
    } catch (err) {
      toast.error(getApiError(err))
    }
  }

  const handleDelete = async (exit: Exit) => {
    const ok = await confirm({
      title: 'Delete Exit',
      message: `Delete exit "${exit.name}"? This action cannot be undone.`,
      confirmText: 'Delete',
      dangerous: true,
    })
    if (ok) deleteMut.mutate(exit.id)
  }

  const uniqueCopyName = (name: string) => {
    const existing = new Set(exits.map(exit => exit.name))
    let candidate = `${name}-copy`
    let suffix = 2
    while (existing.has(candidate)) {
      candidate = `${name}-copy-${suffix}`
      suffix += 1
    }
    return candidate
  }

  const copyMut = useMutation({
    mutationFn: async (exit: Exit) => {
      const config = await getExitConfig(exit.id)
      return createExit({
        name: uniqueCopyName(exit.name),
        enabled: false,
        config_yaml: buildComposeYaml(false, extractEnvironmentBlock(config.config_yaml)),
      })
    },
    onSuccess: async () => { await invalidate(); toast.success('Exit copied') },
    onError: err => toast.error(getApiError(err)),
  })

  return (
    <div className="space-y-6">
      {confirmModal}

      <div className="page-header">
        <div>
          <h1 className="page-title">Exits</h1>
          <p className="page-subtitle">Manage Gluetun VPN exit containers</p>
        </div>
        <button className="btn-primary" onClick={() => setShowForm(true)}>
          <Plus size={15} /> New Exit
        </button>
      </div>

      {isLoading ? (
        <div className="card flex items-center justify-center py-16">
          <div className="text-gray-500 text-sm flex items-center gap-2">
            <svg className="animate-spin h-4 w-4" viewBox="0 0 24 24" fill="none">
              <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
              <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
            </svg>
            Loading exits...
          </div>
        </div>
      ) : exits.length === 0 ? (
        <div className="card">
          <div className="empty-state">
            <div className="w-14 h-14 rounded-2xl bg-gray-800 flex items-center justify-center mb-4">
              <Globe size={24} className="text-gray-600" />
            </div>
            <h3 className="text-gray-300 font-medium mb-1">No exits configured</h3>
            <p className="text-gray-500 text-sm mb-4">Add a VPN exit to start routing traffic</p>
            <button className="btn-primary" onClick={() => setShowForm(true)}>
              <Plus size={15} /> Add First Exit
            </button>
          </div>
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
              onCopy={() => copyMut.mutate(exit)}
              onDelete={() => handleDelete(exit)}
              onLogs={() => handleLogs(exit)}
            />
          ))}
        </div>
      )}

      {(showForm || editExit) && (
        <ExitFormModal
          exit={editExit}
          onClose={() => { setShowForm(false); setEditExit(null) }}
          onSubmit={data => {
            if (editExit) updateMut.mutate({ id: editExit.id, data })
            else createMut.mutate(data)
          }}
          loading={createMut.isPending || updateMut.isPending}
          error={(createMut.error && getApiError(createMut.error)) || (updateMut.error && getApiError(updateMut.error)) || ''}
        />
      )}

      {/* Logs Modal */}
      {logModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-sm">
          <div className="bg-gray-900 border border-gray-700 rounded-2xl w-full max-w-3xl shadow-2xl flex flex-col max-h-[80vh]">
            <div className="flex items-center justify-between px-5 py-4 border-b border-gray-800 flex-shrink-0">
              <div className="flex items-center gap-2">
                <Terminal size={16} className="text-green-400" />
                <h3 className="font-semibold text-white">Logs — {logModal.exit.name}</h3>
              </div>
              <button className="btn-ghost p-1" onClick={() => setLogModal(null)}>
                <X size={16} />
              </button>
            </div>
            <pre className="p-5 text-xs font-mono text-green-300 bg-black/60 overflow-auto flex-1 whitespace-pre-wrap break-all leading-relaxed">
              {logModal.logs || '(no logs available)'}
            </pre>
          </div>
        </div>
      )}
    </div>
  )
}

// ─── Exit Card ────────────────────────────────────────────────────────────────

function ExitCard({
  exit, onStart, onStop, onRestart, onEdit, onCopy, onDelete, onLogs,
}: {
  exit: Exit
  onStart: () => void
  onStop: () => void
  onRestart: () => void
  onEdit: () => void
  onCopy: () => void
  onDelete: () => void
  onLogs: () => void
}) {
  return (
    <div className="card hover:border-gray-700/80 transition-colors">
      <div className="flex items-start justify-between gap-4">
        <div className="flex items-start gap-3">
          <div className="mt-1.5">
            <StatusDot status={exit.status} />
          </div>
          <div>
            <div className="flex items-center gap-2 mb-1">
              <span className="font-semibold text-white">{exit.name}</span>
              <StatusBadge status={exit.status} />
              {!exit.enabled && <span className="badge-gray">Disabled</span>}
            </div>
            <div className="text-xs text-gray-500 flex items-center gap-2 flex-wrap">
              <span className="flex items-center gap-1">
                {exit.protocol === 'wireguard' ? <Wifi size={11} /> : <Shield size={11} />}
                {exit.protocol}
              </span>
              <span>·</span>
              <span>{exit.provider}</span>
              <span>·</span>
              <span>{exit.country}{exit.city ? ` (${exit.city})` : ''}</span>
            </div>
            {exit.public_ip && (
              <div className="text-xs font-mono text-indigo-400 mt-1.5 flex items-center gap-1">
                <span className="w-1.5 h-1.5 rounded-full bg-indigo-400 inline-block" />
                {exit.public_ip}
              </div>
            )}
          </div>
        </div>

        <div className="flex items-center gap-1 flex-wrap justify-end flex-shrink-0">
          {exit.status !== 'running' ? (
            <button className="btn-success text-xs" onClick={onStart}>
              <Play size={12} /> Start
            </button>
          ) : (
            <button className="btn-secondary text-xs" onClick={onStop}>
              <Square size={12} /> Stop
            </button>
          )}
          <button className="btn-secondary text-xs" onClick={onRestart}>
            <RotateCcw size={12} /> Restart
          </button>
          <button className="btn-ghost text-xs" onClick={onEdit}>
            <Pencil size={12} /> Edit
          </button>
          <button className="btn-ghost text-xs" onClick={onCopy}>
            <Copy size={12} /> Copy
          </button>
          <button className="btn-ghost text-xs" onClick={onLogs}>
            <Terminal size={12} /> Logs
          </button>
          <button
            className="btn-ghost text-xs text-red-500 hover:text-red-400 hover:bg-red-950/30"
            onClick={onDelete}
          >
            <Trash2 size={12} />
          </button>
        </div>
      </div>
    </div>
  )
}

// ─── Exit Form Modal ──────────────────────────────────────────────────────────

function ExitFormModal({
  exit, onClose, onSubmit, loading, error,
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
      VPN_SERVICE_PROVIDER: 'protonvpn', VPN_TYPE: 'wireguard',
      SERVER_COUNTRIES: 'United States', SERVER_CITIES: '',
      WIREGUARD_PRIVATE_KEY: '', WIREGUARD_ADDRESSES: '',
    }, 'wireguard'),
  })
  const [localError, setLocalError] = useState('')
  const [showPreview, setShowPreview] = useState(false)

  const env = parseEnvYamlBlock(form.environment_yaml)
  const knownFields = getOrderedEnvKeys(env, form.protocol)
  const knownFieldSet = new Set(knownFields)
  const customFields = Object.keys(env).filter(key => !knownFieldSet.has(key))

  const updateEnvField = (key: string, value: string) => {
    const next = parseEnvYamlBlock(form.environment_yaml)
    next[key] = value
    if (!next.VPN_TYPE) next.VPN_TYPE = form.protocol
    setForm(f => ({ ...f, environment_yaml: stringifyEnvYamlBlock(next, f.protocol) }))
  }

  const renameEnvField = (oldKey: string, rawKey: string) => {
    const newKey = normalizeEnvKey(rawKey)
    if (!newKey || newKey === oldKey) return
    const next = parseEnvYamlBlock(form.environment_yaml)
    const value = next[oldKey] ?? ''
    delete next[oldKey]
    next[newKey] = value
    if (!next.VPN_TYPE) next.VPN_TYPE = form.protocol
    setForm(f => ({ ...f, environment_yaml: stringifyEnvYamlBlock(next, f.protocol) }))
  }

  const removeEnvField = (key: string) => {
    const next = parseEnvYamlBlock(form.environment_yaml)
    delete next[key]
    if (!next.VPN_TYPE) next.VPN_TYPE = form.protocol
    setForm(f => ({ ...f, environment_yaml: stringifyEnvYamlBlock(next, f.protocol) }))
  }

  const addEnvField = () => {
    const next = parseEnvYamlBlock(form.environment_yaml)
    next[nextCustomEnvKey(next)] = ''
    if (!next.VPN_TYPE) next.VPN_TYPE = form.protocol
    setForm(f => ({ ...f, environment_yaml: stringifyEnvYamlBlock(next, f.protocol) }))
  }

  const handleImportFile = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    if (!file) return
    const content = await file.text()
    const importedEnv = /\[(interface|peer)]/i.test(content)
      ? parseWireguardConfig(content)
      : parseEnvYamlBlock(content)
    const protocol = importedEnv.VPN_TYPE === 'openvpn' ? 'openvpn' : 'wireguard'
    setForm(f => ({ ...f, protocol, environment_yaml: stringifyEnvYamlBlock(importedEnv, protocol) }))
    setLocalError('')
    event.target.value = ''
  }

  useEffect(() => {
    if (!exit || !configData?.config_yaml) return
    const environmentYaml = extractEnvironmentBlock(configData.config_yaml)
    const env = parseEnvYamlBlock(environmentYaml)
    const protocol = env.VPN_TYPE === 'openvpn' ? 'openvpn' : 'wireguard'
    setForm({ name: exit.name, enabled: exit.enabled, protocol, environment_yaml: stringifyEnvYamlBlock(env, protocol) })
    setLocalError('')
  }, [configData, exit])

  const handleSubmit = () => {
    const err = validateExitForm(form.name, form.protocol, form.environment_yaml)
    if (err) { setLocalError(err); return }
    setLocalError('')
    onSubmit({ name: form.name, enabled: form.enabled, config_yaml: buildComposeYaml(form.enabled, form.environment_yaml) })
  }

  const displayError = localError || error

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-sm">
      <div className="bg-gray-900 border border-gray-700 rounded-2xl w-full max-w-lg shadow-2xl flex flex-col max-h-[90vh]">
        {/* Header */}
        <div className="flex items-center justify-between px-5 py-4 border-b border-gray-800 flex-shrink-0">
          <h3 className="font-semibold text-white">{exit ? 'Edit Exit' : 'New Exit'}</h3>
          <button className="btn-ghost p-1" onClick={onClose}><X size={16} /></button>
        </div>

        {/* Body */}
        <div className="p-5 space-y-4 overflow-y-auto flex-1">
          {/* Name + enabled */}
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="label">Exit Name *</label>
              <input
                className="input"
                placeholder="us"
                value={form.name}
                onChange={e => setForm(f => ({ ...f, name: e.target.value }))}
              />
            </div>
            <div className="flex items-end pb-1">
              <label className="flex items-center gap-2 cursor-pointer select-none">
                <div
                  className={`relative w-10 h-6 rounded-full transition-colors ${
                    form.enabled ? 'bg-indigo-600' : 'bg-gray-700'
                  }`}
                  onClick={() => setForm(f => ({ ...f, enabled: !f.enabled }))}
                >
                  <div className={`absolute top-1 w-4 h-4 rounded-full bg-white shadow transition-all ${
                    form.enabled ? 'left-5' : 'left-1'
                  }`} />
                </div>
                <span className="text-sm text-gray-300">Enabled</span>
              </label>
            </div>
          </div>

          {/* Protocol toggle */}
          <div>
            <label className="label">VPN Protocol</label>
            <div className="flex gap-2">
              <button
                type="button"
                className={form.protocol === 'wireguard' ? 'btn-primary' : 'btn-secondary'}
                onClick={() => setForm(f => ({
                  ...f, protocol: 'wireguard',
                  environment_yaml: convertEnvironmentBlock(f.environment_yaml, 'wireguard'),
                }))}
              >
                <Wifi size={14} /> WireGuard
              </button>
              <button
                type="button"
                className={form.protocol === 'openvpn' ? 'btn-primary' : 'btn-secondary'}
                onClick={() => setForm(f => ({
                  ...f, protocol: 'openvpn',
                  environment_yaml: convertEnvironmentBlock(f.environment_yaml, 'openvpn'),
                }))}
              >
                <Shield size={14} /> OpenVPN
              </button>
            </div>
          </div>

          {/* Import file */}
          <div>
            <label className="label">Import Config File</label>
            <input
              className="input text-xs"
              type="file"
              accept=".env,.txt,.conf,.ini,.wg"
              onChange={handleImportFile}
            />
            <p className="text-xs text-gray-600 mt-1">Supports KEY=VALUE and WireGuard wg0.conf files</p>
          </div>

          {/* Environment fields */}
          <div>
            <label className="label">Environment Variables</label>
            <div className="space-y-3 bg-gray-800/40 rounded-xl p-4 border border-gray-700/50">
              {configLoading && (
                <p className="text-xs text-gray-500">Loading current config...</p>
              )}
              {knownFields.map(key => (
                <div key={key}>
                  <div className="flex items-center justify-between gap-3">
                    <label className="label">{envLabels[key] ?? key}</label>
                    {key === 'VPN_SERVICE_PROVIDER' && (
                      <button
                        type="button"
                        className="text-xs text-indigo-400 hover:text-indigo-300"
                        onClick={() => {
                          const next = applyCustomWireguardDefaults(parseEnvYamlBlock(form.environment_yaml))
                          setForm(f => ({
                            ...f,
                            protocol: 'wireguard',
                            environment_yaml: stringifyEnvYamlBlock(next, 'wireguard'),
                          }))
                        }}
                      >
                        Use Custom
                      </button>
                    )}
                  </div>
                  <input
                    className="input font-mono text-xs"
                    type={key.includes('PASSWORD') || key.includes('PRIVATE_KEY') ? 'password' : 'text'}
                    value={env[key] ?? ''}
                    placeholder={key}
                    onChange={e => updateEnvField(key, e.target.value)}
                  />
                </div>
              ))}
              {customFields.map(key => (
                <div key={key} className="grid grid-cols-[minmax(0,0.9fr)_minmax(0,1fr)_auto] gap-2 items-end">
                  <div>
                    <label className="label">Key</label>
                    <input
                      className="input font-mono text-xs"
                      value={key}
                      onChange={e => renameEnvField(key, e.target.value)}
                    />
                  </div>
                  <div>
                    <label className="label">Value</label>
                    <input
                      className="input font-mono text-xs"
                      type={key.includes('PASSWORD') || key.includes('PRIVATE_KEY') ? 'password' : 'text'}
                      value={env[key] ?? ''}
                      onChange={e => updateEnvField(key, e.target.value)}
                    />
                  </div>
                  <button
                    type="button"
                    className="btn-ghost p-2 mb-0.5 text-red-500 hover:text-red-400 hover:bg-red-950/30"
                    onClick={() => removeEnvField(key)}
                    title="Remove variable"
                  >
                    <Trash2 size={13} />
                  </button>
                </div>
              ))}
              <button type="button" className="btn-secondary text-xs" onClick={addEnvField}>
                <Plus size={13} /> Add Variable
              </button>
            </div>
            <p className="text-xs text-gray-600 mt-1.5">
              Custom WireGuard requires endpoint IP/port, server public key, private key, and addresses
            </p>
          </div>

          {/* Compose preview toggle */}
          <div>
            <button
              type="button"
              className="btn-ghost text-xs"
              onClick={() => setShowPreview(v => !v)}
            >
              {showPreview ? 'Hide' : 'Show'} Compose Preview
            </button>
            {showPreview && (
              <pre className="mt-2 input min-h-[140px] font-mono text-xs text-gray-400 whitespace-pre-wrap">
                {buildComposeYaml(form.enabled, form.environment_yaml)}
              </pre>
            )}
          </div>

          {displayError && (
            <div className="text-red-400 text-sm bg-red-950/40 border border-red-800/50 px-3 py-2.5 rounded-xl">
              {displayError}
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="flex justify-end gap-2 px-5 py-4 border-t border-gray-800 flex-shrink-0">
          <button className="btn-secondary" onClick={onClose}>Cancel</button>
          <button
            className="btn-primary"
            disabled={loading || !form.name}
            onClick={handleSubmit}
          >
            {loading ? 'Saving...' : exit ? 'Update Exit' : 'Create Exit'}
          </button>
        </div>
      </div>
    </div>
  )
}
