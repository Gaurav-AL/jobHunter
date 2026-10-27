import { auth, authReady } from './firebase'

const rawApiBaseUrl = import.meta.env.VITE_API_BASE_URL || ''
export const API_BASE_URL = rawApiBaseUrl.replace(/\/$/, '')
export const apiUrl = (path: string) => `${API_BASE_URL}${path.startsWith('/') ? path : `/${path}`}`

const SESSION_TOKEN_KEY = 'jobhunterSessionToken'
const SESSION_EXP_KEY = 'jobhunterSessionExp'

export const getSessionToken = (): string | null => {
  const token = localStorage.getItem(SESSION_TOKEN_KEY)
  const exp = Number(localStorage.getItem(SESSION_EXP_KEY) || '0')
  if (token && exp > Date.now()) {
    return token
  }
  return null
}

export const setSessionToken = (token: string, expiresAtUnixSeconds: number) => {
  localStorage.setItem(SESSION_TOKEN_KEY, token)
  localStorage.setItem(SESSION_EXP_KEY, String(expiresAtUnixSeconds * 1000))
}

export const clearSessionToken = () => {
  localStorage.removeItem(SESSION_TOKEN_KEY)
  localStorage.removeItem(SESSION_EXP_KEY)
}

/**
 * Exchange Firebase ID token for a 2-hour backend Session JWT
 */
export const exchangeSessionToken = async (firebaseIdToken?: string): Promise<string> => {
  await authReady
  const user = auth.currentUser
  if (!user) throw new Error('Please sign in again.')
  const fbToken = firebaseIdToken || await user.getIdToken()

  const res = await fetch(apiUrl('/api/auth/session'), {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${fbToken}`,
    },
    body: JSON.stringify({
      email: user.email || '',
      firebase_uid: user.uid,
    }),
  })

  if (!res.ok) {
    return fbToken
  }
  const data = await res.json().catch(() => ({}))
  if (data.session_token && data.expires_at) {
    setSessionToken(data.session_token, data.expires_at)
    return data.session_token
  }
  return fbToken
}

// Attach a 2-hour session JWT (or fresh Firebase token) to every authenticated API request.
export const apiFetch = async (path: string, init: RequestInit = {}) => {
  if (path.startsWith('/api/public/') || path === '/api/auth/session') {
    return fetch(apiUrl(path), init)
  }

  let token = getSessionToken()
  if (!token) {
    try {
      token = await exchangeSessionToken()
    } catch {
      await authReady
      const user = auth.currentUser
      if (!user) throw new Error('Please sign in again.')
      token = await user.getIdToken()
    }
  }

  const headers = new Headers(init.headers)
  headers.set('Authorization', `Bearer ${token}`)
  const response = await fetch(apiUrl(path), { ...init, headers })

  // If 401 Unauthorized, session token may have expired: clear it and re-exchange once
  if (response.status === 401 && getSessionToken()) {
    clearSessionToken()
    try {
      const freshToken = await exchangeSessionToken()
      const retryHeaders = new Headers(init.headers)
      retryHeaders.set('Authorization', `Bearer ${freshToken}`)
      return fetch(apiUrl(path), { ...init, headers: retryHeaders })
    } catch {
      // Re-exchange failed; propagate 401
    }
  }

  return response
}
