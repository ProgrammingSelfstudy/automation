// Global token/signing-key/headers, cached in the browser only (localStorage)
// so they survive page reloads without ever being sent to or stored on our
// backend.

const TOKEN_STORAGE_KEY = 'interface-lib:global-token'
const SIGN_KEY_STORAGE_KEY = 'interface-lib:global-sign-key'
const HEADERS_STORAGE_KEY = 'interface-lib:global-headers'

function load(key: string): string {
  try {
    return localStorage.getItem(key) ?? ''
  } catch {
    return ''
  }
}

function save(key: string, value: string) {
  try {
    if (value === '') {
      localStorage.removeItem(key)
    } else {
      localStorage.setItem(key, value)
    }
  } catch {
    // localStorage can throw in private-browsing/quota-exceeded situations；
    // losing the cached convenience value isn't worth surfacing an error for.
  }
}

export function loadGlobalToken(): string {
  return load(TOKEN_STORAGE_KEY)
}

export function saveGlobalToken(value: string) {
  save(TOKEN_STORAGE_KEY, value)
}

export function loadGlobalSignKey(): string {
  return load(SIGN_KEY_STORAGE_KEY)
}

export function saveGlobalSignKey(value: string) {
  save(SIGN_KEY_STORAGE_KEY, value)
}

// loadGlobalHeaders returns headers to merge as a base layer under every
// interface's own headers (Authorization/deviceId/os/X-Lang and the like —
// fields that stay the same across the whole API instead of being retyped
// into every single interface).
export function loadGlobalHeaders(): Record<string, string> {
  const raw = load(HEADERS_STORAGE_KEY)
  if (raw === '') {
    return {}
  }
  try {
    const parsed: unknown = JSON.parse(raw)
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed as Record<string, string>
    }
    return {}
  } catch {
    return {}
  }
}

export function saveGlobalHeaders(headers: Record<string, string>) {
  const entries = Object.entries(headers).filter(([key]) => key.trim() !== '')
  save(HEADERS_STORAGE_KEY, entries.length === 0 ? '' : JSON.stringify(Object.fromEntries(entries)))
}
