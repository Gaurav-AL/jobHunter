import { auth, authReady } from './firebase'

const rawApiBaseUrl = import.meta.env.VITE_API_BASE_URL || ''
export const API_BASE_URL = rawApiBaseUrl.replace(/\/$/, '')
export const apiUrl = (path: string) => `${API_BASE_URL}${path.startsWith('/') ? path : `/${path}`}`

// Attach a fresh Firebase ID token to every authenticated API request.
export const apiFetch = async (path: string, init: RequestInit = {}) => {
  await authReady
  const user = auth.currentUser
  if (!user) throw new Error('Please sign in again.')
  const token = await user.getIdToken()
  const headers = new Headers(init.headers)
  headers.set('Authorization', `Bearer ${token}`)
  return fetch(apiUrl(path), { ...init, headers })
}
