import { useQuery } from '@tanstack/react-query'
import { getApiError, getVPNLocations, getVPNProviders } from '../api/client'

export default function ProviderLocationPicker({ protocol, provider, country, city, onChange }: {
  protocol: 'wireguard' | 'openvpn'
  provider: string
  country: string
  city: string
  onChange: (provider: string, country: string, city: string) => void
}) {
  const providers = useQuery({ queryKey: ['vpn-providers'], queryFn: getVPNProviders })
  const locations = useQuery({
    queryKey: ['vpn-locations', provider, protocol, country],
    queryFn: () => getVPNLocations(provider, protocol, country),
    enabled: provider !== '' && provider !== 'custom',
  })
  const available = providers.data?.providers.filter(item => (protocol === 'wireguard' ? item.wireguard : item.openvpn) > 0) ?? []
  return <div className="space-y-2 rounded-xl border border-gray-700/50 bg-gray-800/40 p-4">
    <label className="label">VPN provider and location</label>
    <select className="input text-xs" aria-label="VPN provider" value={provider}
      onChange={event => onChange(event.target.value, '', '')}>
      <option value="custom">Custom / use imported server</option>
      {available.map(item => <option key={item.name} value={item.name}>{item.name}</option>)}
    </select>
    {provider !== 'custom' && provider !== '' && <div className="grid grid-cols-2 gap-2">
      <select className="input text-xs" aria-label="VPN country" value={country}
        onChange={event => onChange(provider, event.target.value, '')}>
        <option value="">Choose country</option>
        {(locations.data?.countries ?? []).map(item => <option key={item.name} value={item.name}>{item.name}</option>)}
      </select>
      <select className="input text-xs" aria-label="VPN city" value={city} disabled={!country}
        onChange={event => onChange(provider, country, event.target.value)}>
        <option value="">Any city</option>
        {(locations.data?.cities ?? []).map(item => <option key={item.name} value={item.name}>{item.name}</option>)}
      </select>
    </div>}
    {providers.error && <p className="text-xs text-red-400">Catalog unavailable: {getApiError(providers.error)}</p>}
    {locations.error && <p className="text-xs text-red-400">Could not load locations: {getApiError(locations.error)}</p>}
    {provider !== 'custom' && <p className="text-xs text-gray-400">A server in this location is selected automatically when the exit starts. Your {protocol === 'openvpn' ? '.ovpn profile' : 'WireGuard client config'} is still required.</p>}
  </div>
}
