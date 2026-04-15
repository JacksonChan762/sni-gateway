export interface AuthState {
  token: string
  username: string
  expiresAt: number
}

const tokenKey = 'sni_token'
const usernameKey = 'sni_username'
const expiresAtKey = 'sni_expires_at'
const expirySkewMs = 30_000

function decodeTokenExpiry(token: string): number {
  try {
    const payload = token.split('.')[1]
    if (!payload) return 0
    const normalized = payload.replace(/-/g, '+').replace(/_/g, '/')
    const padded = normalized.padEnd(Math.ceil(normalized.length / 4) * 4, '=')
    const decoded = JSON.parse(atob(padded))
    return typeof decoded.exp === 'number' ? decoded.exp * 1000 : 0
  } catch {
    return 0
  }
}

export function isAuthExpired(auth: Pick<AuthState, 'expiresAt'>) {
  return !auth.expiresAt || auth.expiresAt <= Date.now() + expirySkewMs
}

export function saveAuth(token: string, username: string, expiresAt?: number): AuthState {
  const auth = {
    token,
    username,
    expiresAt: expiresAt ? expiresAt * 1000 : decodeTokenExpiry(token),
  }
  localStorage.setItem(tokenKey, auth.token)
  localStorage.setItem(usernameKey, auth.username)
  localStorage.setItem(expiresAtKey, String(auth.expiresAt))
  return auth
}

export function clearAuth() {
  localStorage.removeItem(tokenKey)
  localStorage.removeItem(usernameKey)
  localStorage.removeItem(expiresAtKey)
}

export function loadAuth(): AuthState | null {
  const token = localStorage.getItem(tokenKey)
  const username = localStorage.getItem(usernameKey)
  if (!token || !username) {
    clearAuth()
    return null
  }

  const storedExpiresAt = Number(localStorage.getItem(expiresAtKey))
  const auth = {
    token,
    username,
    expiresAt: Number.isFinite(storedExpiresAt) && storedExpiresAt > 0
      ? storedExpiresAt
      : decodeTokenExpiry(token),
  }

  if (isAuthExpired(auth)) {
    clearAuth()
    return null
  }

  if (!storedExpiresAt && auth.expiresAt) {
    localStorage.setItem(expiresAtKey, String(auth.expiresAt))
  }
  return auth
}
