import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getApiError, getVPNProviders, searchVPNServers, type VPNServer } from '../api/client'

export default function ProviderServerPicker({ protocol, onSelect }: {
  protocol: 'wireguard' | 'openvpn'
  onSelect: (provider: string, server: VPNServer) => boolean
}) {
  const [provider, setProvider] = useState('')
  const [country, setCountry] = useState('')
  const [query, setQuery] = useState('')
  const [offset, setOffset] = useState(0)
  const [selected, setSelected] = useState('')
  useEffect(() => { setProvider(''); setCountry(''); setOffset(0); setSelected('') }, [protocol])
  const providers = useQuery({ queryKey: ['vpn-providers'], queryFn: getVPNProviders })
  const servers = useQuery({
    queryKey: ['vpn-servers', provider, protocol, country, query, offset],
    queryFn: () => searchVPNServers(provider, protocol, country, query, offset),
    enabled: Boolean(provider),
  })
  const available = providers.data?.providers.filter(item => (protocol === 'wireguard' ? item.wireguard : item.openvpn) > 0) ?? []
  const selectedProvider = available.find(item => item.name === provider)
  const changeProvider = (value: string) => { setProvider(value); setCountry(''); setOffset(0); setSelected('') }
  const changeCountry = (value: string) => { setCountry(value); setOffset(0) }

  return <div className="space-y-2 rounded-xl border border-gray-700/50 bg-gray-800/40 p-4">
    <label className="label">Provider server catalog</label>
    <div className="grid grid-cols-2 gap-2">
      <select className="input text-xs" value={provider} onChange={event => changeProvider(event.target.value)}>
        <option value="">Choose provider</option>
        {available.map(item => <option key={item.name} value={item.name}>{item.name}</option>)}
      </select>
      <select className="input text-xs" value={country} onChange={event => changeCountry(event.target.value)} disabled={!provider}>
        <option value="">All countries</option>
        {(selectedProvider?.countries ?? []).map(item => <option key={item} value={item}>{item}</option>)}
      </select>
    </div>
    {provider && <input className="input text-xs" placeholder="Search city, server, IP…" value={query}
      onChange={event => { setQuery(event.target.value); setOffset(0) }} />}
    {providers.error && <p className="text-xs text-red-400">Catalog unavailable: {getApiError(providers.error)}</p>}
    {servers.error && <p className="text-xs text-red-400">Could not load servers: {getApiError(servers.error)}</p>}
    {servers.isLoading && provider && <p className="text-xs text-gray-400">Loading servers…</p>}
    {servers.data && provider && <>
      <div className="max-h-48 overflow-y-auto space-y-1">
        {servers.data.servers.map(server => <button key={server.id} type="button"
          className="w-full rounded-md px-2 py-1.5 text-left text-xs text-gray-200 hover:bg-gray-700"
          onClick={() => { if (onSelect(provider, server)) setSelected(`${server.name} (${server.ip})`) }}>
          <span className="font-medium">{server.name}</span>
          <span className="text-gray-400"> · {[server.country, server.city || server.region, server.ip].filter(Boolean).join(' · ')}</span>
        </button>)}
        {servers.data.servers.length === 0 && <p className="text-xs text-gray-400">No matching servers</p>}
      </div>
      <div className="flex items-center justify-between text-xs text-gray-400">
        <span>{servers.data.total.toLocaleString()} matching servers</span>
        <div className="flex gap-2">
          <button type="button" className="btn-ghost text-xs" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - 50))}>Previous</button>
          <button type="button" className="btn-ghost text-xs" disabled={offset + 50 >= servers.data.total} onClick={() => setOffset(offset + 50)}>Next</button>
        </div>
      </div>
    </>}
    {selected && <p className="text-xs text-green-400">Selected server: {selected}</p>}
    <p className="text-xs text-gray-500">Server details only. Import your provider's {protocol === 'openvpn' ? '.ovpn profile and credentials' : 'WireGuard client configuration'} before connecting.</p>
  </div>
}
