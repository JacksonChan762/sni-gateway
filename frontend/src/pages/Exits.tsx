import { useEffect, useId, useRef, useState, type ChangeEvent } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useSearchParams } from 'react-router-dom'
import {
  getExits, createExit, updateExit, deleteExit,
  startExit, stopExit, stopAllExits, restartExit, getExitLogs, getExitConfig, getExitStatus, getApiError,
} from '../api/client'
import { type Exit } from '../api/client'
import {
  Plus, Play, Square, RotateCcw, Trash2, Terminal,
  Globe, X, Pencil, Wifi, Shield, Copy, Activity, Search,
} from 'lucide-react'
import { useToast } from '../components/Toast'
import { useConfirm } from '../components/ConfirmModal'
import PublishStatusNotice from '../components/PublishStatusNotice'
import ProviderLocationPicker from '../components/ProviderLocationPicker'

// ─── VPN config helpers ───────────────────────────────────────────────────────

const customWireguardOrder = [
  'VPN_SERVICE_PROVIDER', 'VPN_TYPE',
  'AUTO_SERVER',
  'WIREGUARD_ENDPOINT_IP', 'WIREGUARD_ENDPOINT_PORT',
  'WIREGUARD_PUBLIC_KEY', 'WIREGUARD_PRIVATE_KEY',
  'WIREGUARD_PRESHARED_KEY', 'WIREGUARD_ADDRESSES',
  'WIREGUARD_DNS', 'WIREGUARD_ALLOWED_IPS',
  'WIREGUARD_MTU', 'WIREGUARD_PERSISTENT_KEEPALIVE_INTERVAL',
]
const wireguardRequiredFields = [
  'WIREGUARD_ENDPOINT_IP', 'WIREGUARD_ENDPOINT_PORT',
  'WIREGUARD_PUBLIC_KEY', 'WIREGUARD_PRIVATE_KEY', 'WIREGUARD_ADDRESSES',
]
const openvpnOrder = [
  'VPN_SERVICE_PROVIDER', 'VPN_TYPE', 'SERVER_COUNTRIES', 'SERVER_CITIES',
  'OPENVPN_USER', 'OPENVPN_PASSWORD',
]
const envLabels: Record<string, string> = {
  VPN_SERVICE_PROVIDER: 'VPN Provider',
  VPN_TYPE: 'Protocol',
  AUTO_SERVER: 'Select server automatically',
  SERVER_COUNTRIES: 'Country',
  SERVER_CITIES: 'City',
  WIREGUARD_ENDPOINT_IP: 'Endpoint address',
  WIREGUARD_ENDPOINT_PORT: 'UDP port',
  WIREGUARD_PUBLIC_KEY: 'Server public key',
  WIREGUARD_PRIVATE_KEY: 'Client private key',
  WIREGUARD_PRESHARED_KEY: 'Pre-shared key',
  WIREGUARD_ADDRESSES: 'Tunnel addresses',
  WIREGUARD_DNS: 'VPN DNS',
  WIREGUARD_ALLOWED_IPS: 'Allowed IPs',
  WIREGUARD_MTU: 'MTU (bytes)',
  WIREGUARD_PERSISTENT_KEEPALIVE_INTERVAL: 'Persistent keepalive (seconds)',
  OPENVPN_USER: 'OpenVPN Username',
  OPENVPN_PASSWORD: 'OpenVPN Password',
}
const wireguardFieldHints: Record<string, string> = {
  WIREGUARD_MTU: 'Default is 1408. Try a lower value if large transfers stall.',
  WIREGUARD_PERSISTENT_KEEPALIVE_INTERVAL: 'Off by default. Try 25 seconds if the tunnel drops while idle behind NAT.',
}

function getOrderedEnvKeys(env: Record<string, string>, protocol: 'wireguard' | 'openvpn') {
  if (protocol === 'openvpn') return openvpnOrder
  return customWireguardOrder
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
  singbox:
    environment:
${envLines}
`
}

function extractEnvironmentBlock(configYaml: string) {
  const lines = configYaml.split('\n')
  const envIndex = lines.findIndex(l => l.trim() === 'environment:')
  if (envIndex === -1) {
    return stringifyEnvYamlBlock({
      VPN_SERVICE_PROVIDER: 'custom', VPN_TYPE: 'wireguard',
      WIREGUARD_ENDPOINT_IP: '', WIREGUARD_ENDPOINT_PORT: '',
      WIREGUARD_PUBLIC_KEY: '', WIREGUARD_PRIVATE_KEY: '', WIREGUARD_ADDRESSES: '',
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
  let peers = 0

  for (const rawLine of input.split('\n')) {
    const line = rawLine.replace(/[;#].*$/, '').trim()
    if (!line) continue
    const sectionMatch = line.match(/^\[(.+)]$/)
    if (sectionMatch) {
      section = sectionMatch[1].trim().toLowerCase()
      if (section === 'peer' && ++peers > 1) {
        throw new Error('This profile has multiple peers. Import a client profile with one peer.')
      }
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
      if (key === 'persistentkeepalive') env.WIREGUARD_PERSISTENT_KEEPALIVE_INTERVAL = value
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
  const bracketed = trimmed.match(/^\[([^\[\]]*:[^\[\]]*)\](?::(\d+))?$/)
  if (bracketed) return { host: bracketed[1], port: bracketed[2] ?? '' }
  const hostWithPort = trimmed.match(/^([^:/\s]+):(\d+)$/)
  if (hostWithPort) return { host: hostWithPort[1], port: hostWithPort[2] }
  return { host: trimmed, port: '' }
}

function normalizeWireguardEndpoint(env: Record<string, string>) {
  const { host, port } = parseWireguardEndpoint(env.WIREGUARD_ENDPOINT_IP ?? '')
  return { ...env, WIREGUARD_ENDPOINT_IP: host, WIREGUARD_ENDPOINT_PORT: port || env.WIREGUARD_ENDPOINT_PORT || '' }
}

function validateExitForm(
  name: string,
  providerMode: boolean,
  protocol: 'wireguard' | 'openvpn',
  environmentYaml: string,
  openvpnProfile: string,
  provider: string,
  country: string,
  city: string,
  openvpnUser: string,
  openvpnPassword: string,
  openvpnCert: string,
  openvpnKey: string,
  openvpnKeyPassphrase: string
) {
  if (!/^[a-zA-Z0-9][a-zA-Z0-9_-]{0,62}$/.test(name.trim())) return 'Use 1–63 letters, numbers, hyphens, or underscores for the name'
  if (provider !== 'custom' && !country && !city) return 'Choose a country or city for automatic server selection'
  if (protocol === 'openvpn') {
    if (!providerMode) return openvpnProfile.trim() ? '' : 'Import or paste a complete .ovpn profile'
    const needsUser = !['airvpn', 'vpnsecure'].includes(provider)
    if (needsUser && !openvpnUser.trim()) return 'Enter your provider OpenVPN username'
    if (needsUser && !openvpnPassword.trim() && !(provider === 'ivpn' && /^(i|ivpn)-[a-zA-Z0-9]{4}-[a-zA-Z0-9]{4}-[a-zA-Z0-9]{4}$/.test(openvpnUser.trim()))) return 'Enter your provider OpenVPN password'
    if (['airvpn', 'cyberghost', 'vpnsecure', 'vpn unlimited'].includes(provider) && (!openvpnCert.trim() || !openvpnKey.trim())) return 'This provider also requires your OpenVPN client certificate and key'
    if (provider === 'vpnsecure' && !openvpnKeyPassphrase.trim()) return 'VPN Secure requires the encrypted key passphrase'
    return ''
  }
  const env = parseEnvYamlBlock(environmentYaml)
  if (!providerMode) {
    const missing = wireguardRequiredFields.find(key => !env[key])
    if (missing) return `Enter the ${envLabels[missing].toLowerCase()}.`
    const port = env.WIREGUARD_ENDPOINT_PORT
    if (!/^\d+$/.test(port) || Number(port) < 1 || Number(port) > 65535) return 'Enter a UDP port from 1 to 65535.'
    return ''
  }
  if (!env.WIREGUARD_PRIVATE_KEY) return 'Enter the WireGuard client private key.'
  if (!['protonvpn', 'nordvpn'].includes(provider) && !env.WIREGUARD_ADDRESSES) return 'Enter the tunnel addresses required by this provider.'
  if (provider === 'airvpn' && !env.WIREGUARD_PRESHARED_KEY) return 'Enter the pre-shared key required by AirVPN.'
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

const exitStatusFilters = [
  { value: 'all', label: 'All statuses' },
  { value: 'running', label: 'Running' },
  { value: 'unhealthy', label: 'Unhealthy' },
  { value: 'stopped', label: 'Stopped' },
  { value: 'disabled', label: 'Disabled' },
]

function matchesExitStatus(exit: Exit, status: string) {
  if (status === 'all') return true
  if (status === 'disabled') return !exit.enabled
  if (status === 'stopped') return exit.status === 'stopped' || exit.status === 'not_found'
  return exit.status === status
}

type ExitOperation = 'check' | 'start' | 'stop' | 'restart' | 'save' | 'copy' | 'delete'

// ─── Main component ───────────────────────────────────────────────────────────

export default function Exits() {
  const qc = useQueryClient()
  const toast = useToast()
  const { confirm, modal: confirmModal } = useConfirm()
  const [searchParams, setSearchParams] = useSearchParams()
  const searchInput = useRef<HTMLInputElement>(null)
  const { data: cachedExits, isLoading, error, refetch } = useQuery({
    queryKey: ['exits'],
    queryFn: getExits,
    refetchInterval: 15_000,
  })
  const exits = cachedExits ?? []
  const search = searchParams.get('q') ?? ''
  const status = exitStatusFilters.find(option => option.value === searchParams.get('status'))?.value ?? 'all'
  const filtersActive = search !== '' || status !== 'all'
  const searchTerms = search.toLowerCase().split(/\s+/).filter(Boolean)
  const visibleExits = exits.filter(exit => {
    if (!matchesExitStatus(exit, status)) return false
    const fields = [exit.name, exit.provider, exit.country, exit.city, exit.protocol, exit.public_ip, exit.endpoint_ip]
    const searchable = fields.join(' ').toLowerCase()
    return searchTerms.every(term => searchable.includes(term))
  })
  const [showForm, setShowForm] = useState(false)
  const [logModal, setLogModal] = useState<{ exit: Exit; logs: string } | null>(null)
  const [editExit, setEditExit] = useState<Exit | null>(null)
  const [checkingExitId, setCheckingExitId] = useState<number | null>(null)

  const updateFilter = (key: 'q' | 'status', value: string) => {
    const next = new URLSearchParams(searchParams)
    if (value === '' || (key === 'status' && value === 'all')) next.delete(key)
    else next.set(key, value)
    setSearchParams(next, { replace: key === 'q' })
  }

  const clearFilters = () => {
    const next = new URLSearchParams(searchParams)
    next.delete('q')
    next.delete('status')
    setSearchParams(next)
    searchInput.current?.focus()
  }

  const invalidate = async () => {
    await Promise.all([
      qc.invalidateQueries({ queryKey: ['exits'] }),
      qc.invalidateQueries({ queryKey: ['stats'] }),
      qc.invalidateQueries({ queryKey: ['publish-status'] }),
    ])
  }

  const createMut = useMutation({
    mutationFn: createExit,
    onSuccess: async result => {
      await invalidate()
      setShowForm(false)
      if (result.status === 'unhealthy') toast.error('Exit saved, but VPN connection failed. Check its logs and retry.')
      else toast.success(result.status === 'running' ? `VPN connected (${result.public_ip})` : 'Exit created')
    },
    onError: err => toast.error(getApiError(err)),
  })
  const updateMut = useMutation({
    mutationFn: ({ id, data }: { id: number; data: { name: string; enabled: boolean; protocol?: string; provider?: string; country?: string; city?: string; config_yaml?: string; openvpn_profile?: string; openvpn_user?: string; openvpn_password?: string } }) =>
      updateExit(id, data),
    onSuccess: async (result, vars) => {
      qc.removeQueries({ queryKey: ['exit-config', vars.id] })
      await invalidate()
      setEditExit(null)
      if (result.status === 'unhealthy') toast.error('Exit updated, but VPN connection failed. Check its logs and retry.')
      else toast.success('Exit updated')
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
    onSuccess: async result => {
      await invalidate()
      if (result.status === 'running') toast.success(`VPN connected (${result.public_ip})`)
      else toast.error('VPN connection failed. Check the exit logs and retry.')
    },
    onError: err => toast.error(getApiError(err)),
  })
  const stopMut = useMutation({
    mutationFn: stopExit,
    onSuccess: () => toast.success('Exit stopped'),
    onSettled: invalidate,
    onError: err => toast.error(getApiError(err)),
  })
  const restartMut = useMutation({
    mutationFn: restartExit,
    onSuccess: async result => {
      await invalidate()
      if (result.status === 'running') toast.success(`VPN connected (${result.public_ip})`)
      else toast.error('VPN connection failed after restart. Check the exit logs.')
    },
    onError: err => toast.error(getApiError(err)),
  })

  const [stopFailures, setStopFailures] = useState<string[]>([])
  const stopAllMut = useMutation({
    mutationFn: stopAllExits,
    onSuccess: result => {
      setStopFailures(result.results.filter(r => r.error).map(r => `${r.name}: ${r.error}`))
      if (result.failed_count) toast.error(`${result.failed_count} exits could not be fully stopped`)
      else toast.success('All VPN exits stopped')
    },
    onError: err => toast.error(getApiError(err)),
    onSettled: invalidate,
  })
  const handleStopAll = async () => {
    const ok = await confirm({
      title: 'Stop All Exits',
      message: 'Stop all VPN exits? Routed connections will disconnect. Rules and subscriptions are kept; start exits again to resume.',
      confirmText: 'Stop All',
      dangerous: true,
    })
    if (ok) { setStopFailures([]); stopAllMut.mutate() }
  }

  const handleLogs = async (exit: Exit) => {
    try {
      const res = await getExitLogs(exit.id)
      setLogModal({ exit, logs: res.logs })
    } catch (err) {
      toast.error(getApiError(err))
    }
  }

  const handleCheckStatus = async (exit: Exit) => {
    setCheckingExitId(exit.id)
    try {
      const result = await getExitStatus(exit.id)
      qc.setQueryData<Exit[]>(['exits'], previous => previous?.map(item =>
        item.id === exit.id ? { ...item, status: result.status, public_ip: result.public_ip } : item
      ))
      void qc.invalidateQueries({ queryKey: ['stats'] })
      const message = `${exit.name}: ${result.status}${result.public_ip ? ` (${result.public_ip})` : ''}`
      if (result.status === 'unhealthy') toast.error(message)
      else toast.success(message)
    } catch (err) {
      toast.error(`Could not check ${exit.name}: ${getApiError(err)}`)
    } finally {
      setCheckingExitId(null)
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
      if (exit.provider !== 'custom' && (config.auto_server || exit.runtime === 'gluetun')) {
        const env = parseEnvYamlBlock(extractEnvironmentBlock(config.config_yaml))
        return createExit({
          name: uniqueCopyName(exit.name), runtime: 'sing-box', protocol: exit.protocol, enabled: false,
          auto_server: true,
          provider: exit.provider, country: exit.country, city: exit.city,
          ...(exit.protocol === 'openvpn' ? {
            openvpn_user: config.openvpn_user, openvpn_password: config.openvpn_password,
            openvpn_cert: config.openvpn_cert, openvpn_key: config.openvpn_key,
            openvpn_key_passphrase: config.openvpn_key_passphrase,
          } : {
            wireguard_private_key: env.WIREGUARD_PRIVATE_KEY,
            wireguard_addresses: env.WIREGUARD_ADDRESSES,
            wireguard_preshared_key: env.WIREGUARD_PRESHARED_KEY,
          }),
        })
      }
      if (exit.protocol === 'openvpn') return createExit({
        name: uniqueCopyName(exit.name), runtime: 'sing-box', protocol: 'openvpn', enabled: false,
        provider: exit.provider, country: exit.country, city: exit.city,
        openvpn_profile: config.openvpn_profile, openvpn_user: config.openvpn_user, openvpn_password: config.openvpn_password,
      })
      return createExit({
        name: uniqueCopyName(exit.name),
        enabled: false,
        config_yaml: buildComposeYaml(false, extractEnvironmentBlock(config.config_yaml)),
      })
    },
    onSuccess: async () => { await invalidate(); toast.success('Exit copied') },
    onError: err => toast.error(getApiError(err)),
  })

  const pendingOperations: [ExitOperation, number | null | undefined][] = [
    ['check', checkingExitId],
    ['start', startMut.isPending ? startMut.variables : null],
    ['stop', stopMut.isPending ? stopMut.variables : null],
    ['restart', restartMut.isPending ? restartMut.variables : null],
    ['save', updateMut.isPending ? updateMut.variables?.id : null],
    ['copy', copyMut.isPending ? copyMut.variables?.id : null],
    ['delete', deleteMut.isPending ? deleteMut.variables : null],
  ]
  const runtimeBusy = createMut.isPending || stopAllMut.isPending || pendingOperations.some(([, exitId]) => exitId != null)

  return (
    <div className="space-y-6">
      {confirmModal}

      <div className="page-header">
        <div>
          <h1 className="page-title">Exits</h1>
          <p className="page-subtitle">Manage and check your VPN connections</p>
        </div>
        <div className="flex gap-2">
          <button className="btn-secondary" disabled={runtimeBusy || exits.length === 0} onClick={handleStopAll}>
            <Square size={15} /> {stopAllMut.isPending ? 'Stopping…' : 'Stop All'}
          </button>
          <button className="btn-primary" disabled={runtimeBusy} onClick={() => setShowForm(true)}>
            <Plus size={15} /> New Exit
          </button>
        </div>
      </div>

      <PublishStatusNotice />
      <p className="text-xs text-gray-400">Status updates automatically. Use Check status to run a live check.</p>
      {error && <div role="alert" className="card border-red-800 text-red-300 text-sm">
        Could not {cachedExits ? 'refresh' : 'load'} exits. {getApiError(error)}
        <button className="btn-secondary ml-3" onClick={() => void refetch()}>Retry</button>
        {cachedExits && <p className="mt-2">Showing the last received list.</p>}
      </div>}
      {stopFailures.length > 0 && <div className="card border-red-800" role="alert">
        <p className="text-red-300 mb-2">Some exits could not be stopped. Retry Stop All after checking these errors:</p>
        {stopFailures.map(message => <p key={message} className="text-sm text-red-400">{message}</p>)}
      </div>}
      {cachedExits && (exits.length > 0 || filtersActive) && (
        <div className="card grid gap-3 sm:grid-cols-[minmax(0,1fr)_11rem]">
          <div>
            <label className="label" htmlFor="exit-search">Search exits</label>
            <div className="relative">
              <Search size={15} aria-hidden="true" className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-500" />
              <input ref={searchInput} id="exit-search" type="search" className="input pl-9"
                value={search} placeholder="Name, location, or IP"
                onChange={event => updateFilter('q', event.target.value)} />
            </div>
          </div>
          <div>
            <label className="label" htmlFor="exit-status-filter">Status</label>
            <select id="exit-status-filter" className="select" value={status} onChange={event => updateFilter('status', event.target.value)}>
              {exitStatusFilters.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}
            </select>
          </div>
          <div className="flex flex-wrap items-center justify-between gap-2 sm:col-span-2">
            <p role="status" className="text-xs text-gray-400">
              {filtersActive ? `Showing ${visibleExits.length} of ${exits.length} exits` : `${exits.length} ${exits.length === 1 ? 'exit' : 'exits'}`}
            </p>
            {filtersActive && <button className="btn-ghost text-xs" onClick={clearFilters}><X size={12} /> Clear filters</button>}
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
            Loading exits...
          </div>
        </div>
      ) : error && !cachedExits ? null : exits.length === 0 ? (
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
      ) : visibleExits.length === 0 ? (
        <div className="card text-center py-10">
          <Search size={24} aria-hidden="true" className="mx-auto mb-3 text-gray-500" />
          <h2 className="text-sm font-medium text-gray-200">No matching exits</h2>
          <p className="mt-1 text-sm text-gray-400">Try a different search or status.</p>
          <button className="btn-secondary mt-4" onClick={clearFilters}>Show all exits</button>
        </div>
      ) : (
        <div className="space-y-3">
          {visibleExits.map(exit => (
            <ExitCard
              key={exit.id}
              exit={exit}
              onStart={() => startMut.mutate(exit.id)}
              onStop={() => stopMut.mutate(exit.id)}
              busy={runtimeBusy}
              onRestart={() => restartMut.mutate(exit.id)}
              onEdit={() => setEditExit(exit)}
              onCopy={() => copyMut.mutate(exit)}
              onDelete={() => handleDelete(exit)}
              onLogs={() => handleLogs(exit)}
              onCheck={() => void handleCheckStatus(exit)}
              operation={pendingOperations.find(([, exitId]) => exitId === exit.id)?.[0]}
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
  exit, onStart, onStop, onRestart, onEdit, onCopy, onDelete, onLogs, onCheck, busy, operation,
}: {
  exit: Exit
  onStart: () => void
  busy: boolean
  onStop: () => void
  onRestart: () => void
  onEdit: () => void
  onCopy: () => void
  onDelete: () => void
  onLogs: () => void
  onCheck: () => void
  operation?: ExitOperation
}) {
  const nameId = useId()
  return (
    <article aria-labelledby={nameId} className="card hover:border-gray-700/80 transition-colors">
      <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div className="flex min-w-0 items-start gap-3">
          <div className="mt-1.5 shrink-0">
            <StatusDot status={exit.status} />
          </div>
          <div className="min-w-0">
            <div className="mb-1 flex flex-wrap items-center gap-2">
              <h2 id={nameId} className="min-w-0 font-semibold text-white [overflow-wrap:anywhere]">{exit.name}</h2>
              <StatusBadge status={exit.status} />
              {exit.runtime !== 'sing-box' && exit.runtime !== 'gluetun' && <span className="badge-yellow">Needs VPN config</span>}
              {!exit.enabled && <span className="badge-gray">Disabled</span>}
            </div>
            <div className="text-xs text-gray-500 flex items-center gap-2 flex-wrap">
              <span className="flex items-center gap-1">
                {exit.protocol === 'wireguard' ? <Wifi size={11} /> : <Shield size={11} />}
                {exit.protocol}
              </span>
              <span>·</span>
              <span>{exit.provider}</span>
              {(exit.country || exit.city) && <>
                <span>·</span>
                <span>{[exit.country, exit.city].filter(Boolean).join(' · ')}</span>
              </>}
            </div>
            {exit.public_ip && (
              <div className="mt-1.5 font-mono text-xs text-indigo-400 [overflow-wrap:anywhere]">
                Public IP: {exit.public_ip}
              </div>
            )}
            {exit.endpoint_ip && <div className="mt-1 font-mono text-xs text-gray-400 [overflow-wrap:anywhere]">VPN endpoint: {exit.endpoint_ip}</div>}
          </div>
        </div>

        <div role="group" aria-label={`Actions for ${exit.name}`} className="flex flex-wrap items-center gap-1 lg:max-w-[65%] lg:justify-end">
          <button disabled={busy} className="btn-secondary text-xs" onClick={onCheck}>
            <Activity size={12} className={operation === 'check' ? 'animate-pulse' : ''} /> {operation === 'check' ? 'Checking…' : 'Check status'}
          </button>
          {exit.status !== 'running' && (
            <button disabled={busy || !exit.enabled || !['sing-box', 'gluetun'].includes(exit.runtime)} className="btn-success text-xs" onClick={onStart}>
              <Play size={12} /> {operation === 'start' ? 'Starting…' : 'Start'}
            </button>
          )}
          {exit.status !== 'stopped' && exit.status !== 'not_found' && (
            <button disabled={busy} className="btn-secondary text-xs" onClick={onStop}>
              <Square size={12} /> {operation === 'stop' ? 'Stopping…' : 'Stop'}
            </button>
          )}
          <button disabled={busy || !exit.enabled || !['sing-box', 'gluetun'].includes(exit.runtime)} className="btn-secondary text-xs" onClick={onRestart}>
            <RotateCcw size={12} /> {operation === 'restart' ? 'Restarting…' : 'Restart'}
          </button>
          <button className="btn-ghost text-xs" disabled={busy} onClick={onEdit}>
            <Pencil size={12} /> {operation === 'save' ? 'Saving…' : 'Edit'}
          </button>
          <button className="btn-ghost text-xs" disabled={busy || !['sing-box', 'gluetun'].includes(exit.runtime)} onClick={onCopy}>
            <Copy size={12} /> {operation === 'copy' ? 'Copying…' : 'Copy'}
          </button>
          <button className="btn-ghost text-xs" onClick={onLogs}>
            <Terminal size={12} /> Logs
          </button>
          <button
            className="btn-ghost text-xs text-red-500 hover:text-red-400 hover:bg-red-950/30"
            aria-label={`Delete ${exit.name}`} disabled={busy} onClick={onDelete}
          >
            <Trash2 size={12} /> {operation === 'delete' && 'Deleting…'}
          </button>
        </div>
      </div>
    </article>
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
  const { data: configData, error: configError, isLoading: configLoading, refetch: reloadConfig } = useQuery({
    queryKey: ['exit-config', exit?.id],
    queryFn: () => getExitConfig(exit!.id),
    enabled: Boolean(exit),
  })
  const formId = useId()

  const [form, setForm] = useState({
    name: exit?.name ?? '',
    enabled: exit?.enabled ?? true,
    protocol: 'wireguard' as 'wireguard' | 'openvpn',
    runtime: exit?.runtime ?? 'sing-box',
    openvpn_profile: '',
    openvpn_user: '',
    openvpn_password: '',
    openvpn_cert: '',
    openvpn_key: '',
    openvpn_key_passphrase: '',
    provider: exit?.provider ?? 'custom',
    country: exit?.country ?? '',
    city: exit?.city ?? '',
    auto_server: false,
    environment_yaml: stringifyEnvYamlBlock({
      VPN_SERVICE_PROVIDER: 'custom', VPN_TYPE: 'wireguard',
      WIREGUARD_ENDPOINT_IP: '', WIREGUARD_ENDPOINT_PORT: '',
      WIREGUARD_PUBLIC_KEY: '', WIREGUARD_PRIVATE_KEY: '', WIREGUARD_ADDRESSES: '',
    }, 'wireguard'),
  })
  const [localError, setLocalError] = useState('')
  const [importFeedback, setImportFeedback] = useState<{ type: 'success' | 'warning' | 'error'; text: string } | null>(null)
  const [importing, setImporting] = useState(false)
  const [showPreview, setShowPreview] = useState(false)

  const env = parseEnvYamlBlock(form.environment_yaml)
  const providerMode = form.provider !== 'custom' && (form.auto_server || form.runtime === 'gluetun')
  const knownFieldSet = new Set([...getOrderedEnvKeys(env, form.protocol), 'SERVER_COUNTRIES', 'SERVER_CITIES'])
  const knownFields = getOrderedEnvKeys(env, form.protocol).filter(key => {
    if (['VPN_SERVICE_PROVIDER', 'VPN_TYPE', 'AUTO_SERVER'].includes(key)) return false
    if (!providerMode) return true
    if (key === 'WIREGUARD_PRIVATE_KEY') return true
    if (key === 'WIREGUARD_ADDRESSES') return !['protonvpn', 'nordvpn'].includes(form.provider)
    return key === 'WIREGUARD_PRESHARED_KEY' && form.provider === 'airvpn'
  })
  const customFields = Object.keys(env).filter(key => !knownFieldSet.has(key))
  const wireguardFields = knownFields.filter(key => !['WIREGUARD_ENDPOINT_IP', 'WIREGUARD_ENDPOINT_PORT'].includes(key))
  const optionalWireguardFields = wireguardFields.filter(key => !wireguardRequiredFields.includes(key))

  const updateEnvFields = (updates: Record<string, string>) => {
    setForm(f => {
      const next = { ...parseEnvYamlBlock(f.environment_yaml), ...updates }
      next.VPN_TYPE ||= f.protocol
      return { ...f, environment_yaml: stringifyEnvYamlBlock(next, f.protocol) }
    })
    setLocalError('')
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
    const file = event.currentTarget.files?.[0]
    if (!file) return
    event.currentTarget.value = ''
    setImporting(true)
    setImportFeedback(null)
    let content: string
    try {
      content = await file.text()
    } catch {
      setImportFeedback({ type: 'error', text: `Could not read ${file.name}. Choose the file again.` })
      return
    } finally {
      setImporting(false)
    }
    if (!content.trim()) {
      setImportFeedback({ type: 'error', text: `${file.name} is empty.` })
      return
    }
    if (file.name.toLowerCase().endsWith('.ovpn') || /^\s*(client|remote|proto)\s/m.test(content)) {
      setForm(f => ({ ...f, protocol: 'openvpn', runtime: 'sing-box', provider: 'custom', country: '', city: '', auto_server: false, openvpn_profile: content }))
      setLocalError('')
      setImportFeedback({ type: 'success', text: `Imported ${file.name} as an OpenVPN profile.` })
      return
    }
    let importedEnv: Record<string, string>
    try {
      importedEnv = /^\s*\[\s*(interface|peer)\s*\]/im.test(content)
        ? parseWireguardConfig(content)
        : parseEnvYamlBlock(content)
    } catch (err) {
      setImportFeedback({ type: 'error', text: (err as Error).message })
      return
    }
    if (!wireguardRequiredFields.some(key => importedEnv[key])) {
      setImportFeedback({ type: 'error', text: 'Choose a WireGuard client profile or a file with WireGuard connection variables.' })
      return
    }
    const protocol = 'wireguard'
    importedEnv.VPN_SERVICE_PROVIDER = 'custom'
    importedEnv.VPN_TYPE = 'wireguard'
    importedEnv = normalizeWireguardEndpoint(importedEnv)
    setForm(f => ({ ...f, protocol, runtime: 'sing-box', provider: 'custom', country: '', city: '', auto_server: false,
      environment_yaml: stringifyEnvYamlBlock(importedEnv, protocol) }))
    setLocalError('')
    const missing = wireguardRequiredFields.filter(key => !importedEnv[key])
    setImportFeedback(missing.length ? {
      type: 'warning', text: `Imported ${file.name}. Before saving, fill in ${missing.map(key => envLabels[key].toLowerCase()).join(', ')}.`,
    } : { type: 'success', text: `Imported ${file.name}. Review the settings before saving.` })
  }

  useEffect(() => {
    if (!exit || !configData?.config_yaml) return
    if (configData.protocol === 'openvpn') {
      setForm(f => ({ ...f, name: exit.name, enabled: exit.enabled, protocol: 'openvpn', runtime: exit.runtime,
        provider: exit.provider, country: exit.country, city: exit.city,
        auto_server: configData.auto_server,
        openvpn_profile: configData.openvpn_profile ?? '', openvpn_user: configData.openvpn_user ?? '', openvpn_password: configData.openvpn_password ?? '',
        openvpn_cert: configData.openvpn_cert ?? '', openvpn_key: configData.openvpn_key ?? '',
        openvpn_key_passphrase: configData.openvpn_key_passphrase ?? '' }))
      setLocalError('')
      return
    }
    const environmentYaml = extractEnvironmentBlock(configData.config_yaml)
    const env = parseEnvYamlBlock(environmentYaml)
    const protocol = env.VPN_TYPE === 'openvpn' ? 'openvpn' : 'wireguard'
    setForm(f => ({ ...f, name: exit.name, enabled: exit.enabled, protocol, runtime: exit.runtime,
      provider: exit.provider, country: exit.country, city: exit.city, auto_server: configData.auto_server,
      environment_yaml: stringifyEnvYamlBlock(env, protocol) }))
    setLocalError('')
  }, [configData, exit])

  const handleSubmit = () => {
    if (loading || importing || configLoading || configError) return
    const environmentYaml = form.protocol === 'wireguard' && !providerMode
      ? stringifyEnvYamlBlock(normalizeWireguardEndpoint(env), 'wireguard')
      : form.environment_yaml
    const err = validateExitForm(form.name, providerMode, form.protocol, environmentYaml, form.openvpn_profile, form.provider, form.country, form.city,
      form.openvpn_user, form.openvpn_password, form.openvpn_cert, form.openvpn_key, form.openvpn_key_passphrase)
    if (err) { setLocalError(err); return }
    setLocalError('')
    if (providerMode) {
      const credentials = parseEnvYamlBlock(form.environment_yaml)
      onSubmit({ name: form.name, enabled: form.enabled, runtime: 'sing-box', protocol: form.protocol,
        provider: form.provider, country: form.country, city: form.city, auto_server: true,
        ...(form.protocol === 'wireguard' ? {
          wireguard_private_key: credentials.WIREGUARD_PRIVATE_KEY ?? '',
          wireguard_addresses: credentials.WIREGUARD_ADDRESSES ?? '',
          wireguard_preshared_key: credentials.WIREGUARD_PRESHARED_KEY ?? '',
        } : {
          openvpn_user: form.openvpn_user, openvpn_password: form.openvpn_password,
          openvpn_cert: form.openvpn_cert, openvpn_key: form.openvpn_key, openvpn_key_passphrase: form.openvpn_key_passphrase,
        }),
      })
      return
    }
    if (form.protocol === 'openvpn') {
      onSubmit({ name: form.name, enabled: form.enabled, runtime: 'sing-box', protocol: 'openvpn',
        provider: form.provider, country: form.country, city: form.city,
        auto_server: form.auto_server,
        openvpn_profile: form.openvpn_profile, openvpn_user: form.openvpn_user, openvpn_password: form.openvpn_password })
      return
    }
    onSubmit({ name: form.name, enabled: form.enabled, runtime: 'sing-box', config_yaml: buildComposeYaml(form.enabled, environmentYaml) })
  }

  const displayError = localError || error

  const selectLocation = (provider: string, country: string, city: string) => {
    const auto_server = provider !== 'custom' && provider !== ''
    const next = parseEnvYamlBlock(form.environment_yaml)
    next.VPN_SERVICE_PROVIDER = provider
    next.SERVER_COUNTRIES = country
    next.SERVER_CITIES = city
    next.AUTO_SERVER = String(auto_server)
    setForm(f => ({ ...f, provider, country, city, auto_server, runtime: 'sing-box',
      environment_yaml: stringifyEnvYamlBlock(next, 'wireguard') }))
    setLocalError('')
    setImportFeedback(null)
  }

  const renderWireguardField = (key: string) => {
    const id = `${formId}-${key}`
    const required = providerMode || wireguardRequiredFields.includes(key)
    const secret = key === 'WIREGUARD_PRIVATE_KEY' || key === 'WIREGUARD_PRESHARED_KEY'
    const hint = wireguardFieldHints[key]
    return (
      <div key={key}>
        <label className="label" htmlFor={id}>{envLabels[key] ?? key}{required && ' *'}</label>
        <input
          id={id}
          className="input font-mono text-xs"
          type={secret ? 'password' : 'text'}
          autoComplete={secret ? 'new-password' : 'off'}
          spellCheck={false}
          aria-required={required}
          aria-describedby={hint ? `${id}-hint` : undefined}
          value={env[key] ?? ''}
          placeholder={key === 'WIREGUARD_ADDRESSES' ? '10.2.0.2/32' : undefined}
          onChange={e => updateEnvFields({ [key]: e.target.value })}
        />
        {hint && <p className="mt-1.5 text-xs text-gray-400" id={`${id}-hint`}>{hint}</p>}
      </div>
    )
  }

  const splitEndpointInput = (value: string) => {
    const { host, port } = parseWireguardEndpoint(value)
    if (!port && host === value.trim()) return false
    updateEnvFields({ WIREGUARD_ENDPOINT_IP: host, ...(port && { WIREGUARD_ENDPOINT_PORT: port }) })
    return true
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-sm">
      <div role="dialog" aria-modal="true" aria-labelledby={`${formId}-title`} className="w-full max-w-lg"
        onKeyDown={event => { if (event.key === 'Escape') onClose() }}>
      <form className="bg-gray-900 border border-gray-700 rounded-2xl shadow-2xl flex flex-col max-h-[90vh]" noValidate
        onSubmit={event => { event.preventDefault(); handleSubmit() }}>
        {/* Header */}
        <div className="flex items-center justify-between px-5 py-4 border-b border-gray-800 flex-shrink-0">
          <h3 className="font-semibold text-white" id={`${formId}-title`}>{exit ? 'Edit Exit' : 'New Exit'}</h3>
          <button type="button" className="btn-ghost p-1" aria-label="Close exit form" onClick={onClose}><X size={16} /></button>
        </div>

        {/* Body */}
        <div className="p-5 space-y-4 overflow-y-auto flex-1">
          {configLoading && <p className="text-xs text-gray-400" role="status">Loading current configuration…</p>}
          {configError && <div className="text-sm text-red-300" role="alert">
            Could not load the current configuration.
            <button type="button" className="btn-secondary ml-2 text-xs" onClick={() => void reloadConfig()}>Retry</button>
          </div>}
          {/* Name + enabled */}
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="label" htmlFor={`${formId}-name`}>Exit Name *</label>
              <input
                id={`${formId}-name`}
                className="input"
                placeholder="us"
                autoFocus
                aria-required="true"
                value={form.name}
                onChange={e => { setForm(f => ({ ...f, name: e.target.value })); setLocalError('') }}
              />
            </div>
            <div className="flex items-end pb-1">
              <label className="flex items-center gap-2 cursor-pointer select-none">
                <input type="checkbox" role="switch" className="peer sr-only" checked={form.enabled}
                  onChange={e => setForm(f => ({ ...f, enabled: e.target.checked }))} />
                <span aria-hidden="true" className="relative h-6 w-10 rounded-full bg-gray-700 transition-colors
                  before:absolute before:left-1 before:top-1 before:h-4 before:w-4 before:rounded-full before:bg-white before:shadow before:transition-transform
                  peer-checked:bg-indigo-600 peer-checked:before:translate-x-4 peer-focus-visible:ring-2 peer-focus-visible:ring-indigo-400" />
                <span className="text-sm text-gray-300">Enabled</span>
              </label>
            </div>
          </div>

          {/* Protocol */}
          <div>
            <label className="label">VPN Protocol</label>
            <div className="flex gap-2">
              <button
                type="button"
                className={form.protocol === 'wireguard' ? 'btn-primary' : 'btn-secondary'}
                aria-pressed={form.protocol === 'wireguard'}
                onClick={() => {
                  if (form.protocol === 'wireguard') return
                  setLocalError('')
                  setImportFeedback(null)
                  setForm(f => ({
                    ...f, protocol: 'wireguard', runtime: 'sing-box', provider: 'custom', country: '', city: '', auto_server: false,
                    environment_yaml: stringifyEnvYamlBlock({ ...parseEnvYamlBlock(convertEnvironmentBlock(f.environment_yaml, 'wireguard')),
                      VPN_SERVICE_PROVIDER: 'custom', SERVER_COUNTRIES: '', SERVER_CITIES: '', AUTO_SERVER: 'false' }, 'wireguard'),
                  }))
                }}
              >
                <Wifi size={14} /> WireGuard
              </button>
              <button type="button" className={form.protocol === 'openvpn' ? 'btn-primary' : 'btn-secondary'} aria-pressed={form.protocol === 'openvpn'}
                onClick={() => {
                  if (form.protocol === 'openvpn') return
                  setLocalError('')
                  setImportFeedback(null)
                  setForm(f => ({ ...f, protocol: 'openvpn', runtime: 'sing-box', provider: 'custom', country: '', city: '', auto_server: false }))
                }}>
                <Shield size={14} /> OpenVPN
              </button>
            </div>
          </div>

          {/* Import file */}
          {!providerMode && <div>
            <label className="label" htmlFor={`${formId}-profile`}>Import VPN profile</label>
            <input
              id={`${formId}-profile`}
              className="input text-xs"
              type="file"
              accept=".env,.txt,.conf,.ini,.wg,.ovpn"
              onChange={handleImportFile}
              disabled={importing || configLoading}
              aria-describedby={`${formId}-import-help`}
            />
            <p className="mt-1.5 text-xs text-gray-400" id={`${formId}-import-help`}>WireGuard .conf or an OpenVPN .ovpn with inline certificates.</p>
            {importing && <p role="status" className="mt-1.5 text-xs text-gray-400">Reading profile…</p>}
            {importFeedback && <p role={importFeedback.type === 'error' ? 'alert' : 'status'}
              className={`mt-1.5 break-words text-xs ${importFeedback.type === 'success' ? 'text-green-300' : importFeedback.type === 'warning' ? 'text-amber-300' : 'text-red-300'}`}>
              {importFeedback.text}
            </p>}
          </div>}

          {/* Environment fields */}
          <ProviderLocationPicker protocol={form.protocol} provider={form.provider} country={form.country}
            city={form.city} providerMode={providerMode} onChange={selectLocation} />
          {form.protocol === 'openvpn' ? <div className="space-y-3">
            {!providerMode && <div>
              <label className="label">OpenVPN profile *</label>
              <textarea className="input font-mono text-xs min-h-[180px]" value={form.openvpn_profile}
                placeholder={'client\ndev tun\nremote 203.0.113.10 1194\n<ca>\n...\n</ca>'}
                onChange={e => setForm(f => ({ ...f, openvpn_profile: e.target.value }))} />
              <p className="text-xs text-gray-600 mt-1">Embed CA, client certificate, and keys in the profile. External file paths are not supported.</p>
            </div>}
            {(!providerMode || !['airvpn', 'vpnsecure'].includes(form.provider)) && <div><label className="label">OpenVPN username {!providerMode ? '(if required)' : '*'}</label><input className="input" value={form.openvpn_user}
              onChange={e => setForm(f => ({ ...f, openvpn_user: e.target.value }))} /></div>}
            {(!providerMode || !['airvpn', 'vpnsecure'].includes(form.provider)) && <div><label className="label">OpenVPN password {!providerMode || form.provider === 'ivpn' ? '(if required)' : '*'}</label><input className="input" type="password" value={form.openvpn_password}
              onChange={e => setForm(f => ({ ...f, openvpn_password: e.target.value }))} /></div>}
            {providerMode && ['airvpn', 'cyberghost', 'vpnsecure', 'vpn unlimited'].includes(form.provider) && <>
              <div><label className="label">Client certificate *</label><textarea className="input font-mono text-xs min-h-24" value={form.openvpn_cert}
                placeholder="Paste PEM certificate" onChange={e => setForm(f => ({ ...f, openvpn_cert: e.target.value }))} /></div>
              <div><label className="label">{form.provider === 'vpnsecure' ? 'Encrypted client key *' : 'Client key *'}</label><textarea className="input font-mono text-xs min-h-24" value={form.openvpn_key}
                placeholder="Paste PEM private key" onChange={e => setForm(f => ({ ...f, openvpn_key: e.target.value }))} /></div>
              <div><label className="label">Key passphrase {form.provider === 'vpnsecure' ? '*' : '(if encrypted)'}</label><input className="input" type="password" value={form.openvpn_key_passphrase}
                onChange={e => setForm(f => ({ ...f, openvpn_key_passphrase: e.target.value }))} /></div>
            </>}
          </div> : <section aria-labelledby={`${formId}-wireguard`}>
            <h4 className="label" id={`${formId}-wireguard`}>WireGuard connection</h4>
            <div className="space-y-3 bg-gray-800/40 rounded-xl p-4 border border-gray-700/50">
              {!providerMode && <div>
                <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_7rem]">
                  <div>
                    <label className="label" htmlFor={`${formId}-endpoint`}>Endpoint address *</label>
                    <input
                      id={`${formId}-endpoint`}
                      className="input font-mono text-xs"
                      aria-required="true"
                      aria-describedby={`${formId}-endpoint-help`}
                      autoCapitalize="none"
                      autoComplete="off"
                      spellCheck={false}
                      placeholder="vpn.example.com"
                      value={env.WIREGUARD_ENDPOINT_IP ?? ''}
                      onChange={e => updateEnvFields({ WIREGUARD_ENDPOINT_IP: e.target.value })}
                      onPaste={e => { if (splitEndpointInput(e.clipboardData.getData('text/plain'))) e.preventDefault() }}
                      onBlur={e => splitEndpointInput(e.currentTarget.value)}
                    />
                  </div>
                  <div>
                    <label className="label" htmlFor={`${formId}-port`}>UDP port *</label>
                    <input id={`${formId}-port`} className="input font-mono text-xs" inputMode="numeric"
                      aria-required="true" placeholder="51820" value={env.WIREGUARD_ENDPOINT_PORT ?? ''}
                      onChange={e => updateEnvFields({ WIREGUARD_ENDPOINT_PORT: e.target.value })} />
                  </div>
                </div>
                <p className="mt-1.5 text-xs text-gray-400" id={`${formId}-endpoint-help`}>
                  Use an IP address or DDNS hostname. Paste <code>vpn.example.com:51820</code> or <code>[2001:db8::1]:51820</code> to fill both fields.
                </p>
              </div>}
              {wireguardFields.filter(key => providerMode || wireguardRequiredFields.includes(key)).map(renderWireguardField)}
              {!providerMode && <details className="border-t border-gray-700/50 pt-3">
                <summary className="cursor-pointer rounded font-medium text-sm text-gray-300 hover:text-white">
                  Advanced settings
                </summary>
                <div className="mt-3 space-y-3">
                  {optionalWireguardFields.map(renderWireguardField)}
                  {customFields.map(key => (
                    <div key={key} className="grid grid-cols-[minmax(0,0.9fr)_minmax(0,1fr)_auto] gap-2 items-end">
                      <div>
                        <label className="label" htmlFor={`${formId}-${key}-custom-key`}>Key</label>
                        <input id={`${formId}-${key}-custom-key`} className="input font-mono text-xs" defaultValue={key}
                          onBlur={e => renameEnvField(key, e.target.value)} />
                      </div>
                      <div>
                        <label className="label" htmlFor={`${formId}-${key}-custom-value`}>Value</label>
                        <input id={`${formId}-${key}-custom-value`} className="input font-mono text-xs"
                          type={key.includes('PASSWORD') || key.includes('PRIVATE_KEY') ? 'password' : 'text'}
                          value={env[key] ?? ''}
                          onChange={e => updateEnvFields({ [key]: e.target.value })} />
                      </div>
                      <button type="button" className="btn-ghost p-2 mb-0.5 text-red-500 hover:text-red-400 hover:bg-red-950/30"
                        onClick={() => removeEnvField(key)} aria-label={`Remove ${key}`} title="Remove variable">
                        <Trash2 size={13} />
                      </button>
                    </div>
                  ))}
                  <button type="button" className="btn-secondary text-xs" onClick={addEnvField}>
                    <Plus size={13} /> Add Variable
                  </button>
                </div>
              </details>}
            </div>
            {providerMode && <p className="text-xs text-gray-400 mt-1.5">The provider catalog supplies the server endpoint and port.</p>}
          </section>}

          {/* Configuration preview toggle */}
          {form.protocol === 'wireguard' && <div>
            <button
              type="button"
              className="btn-ghost text-xs"
              aria-expanded={showPreview}
              onClick={() => setShowPreview(v => !v)}
            >
              {showPreview ? 'Hide' : 'Show'} Configuration Preview
            </button>
            {showPreview && (
              <pre className="mt-2 input min-h-[140px] font-mono text-xs text-gray-400 whitespace-pre-wrap">
                {buildComposeYaml(form.enabled, form.environment_yaml)}
              </pre>
            )}
          </div>}

        </div>

        {/* Footer */}
        <div className="space-y-3 px-5 py-4 border-t border-gray-800 flex-shrink-0">
          {displayError && <p role="alert" className="text-red-300 text-sm bg-red-950/40 border border-red-800/50 px-3 py-2.5 rounded-xl">
            {displayError}
          </p>}
          <div className="flex justify-end gap-2">
            <button type="button" className="btn-secondary" onClick={onClose}>Cancel</button>
            <button type="submit" className="btn-primary" disabled={loading || importing || configLoading || Boolean(configError)}>
              {loading ? 'Saving...' : exit ? 'Update Exit' : 'Create Exit'}
            </button>
          </div>
        </div>
      </form>
      </div>
    </div>
  )
}
