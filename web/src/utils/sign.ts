import { md5 } from './md5'

// Mirrors AppServerSigner.sign() on the backend: sort params by key, drop
// empty values and the literal string "null", join as raw (unencoded)
// key=value pairs, then append nonce + timestamp + secretKey and MD5+upper.
export function buildSignParams(params: Record<string, string>): string {
  return Object.keys(params)
    .sort()
    .filter((key) => {
      const value = params[key]
      return value !== undefined && value !== null && value !== '' && value !== 'null'
    })
    .map((key) => `${key}=${params[key]}`)
    .join('&')
}

export function computeAppServerSign(
  params: Record<string, string>,
  nonce: string,
  timestamp: string,
  secretKey: string,
): string {
  const signText = buildSignParams(params) + (nonce ?? '') + (timestamp ?? '') + (secretKey ?? '')
  return md5(signText).toUpperCase()
}

// parseFormBody turns a rendered application/x-www-form-urlencoded body back
// into a param map, so the signature can be computed without requiring the
// interface's body_tpl to already be written in sorted-key order.
export function parseFormBody(resolvedBody: string): Record<string, string> {
  const result: Record<string, string> = {}
  for (const pair of resolvedBody.split('&')) {
    if (pair === '') {
      continue
    }
    const eq = pair.indexOf('=')
    if (eq < 0) {
      continue
    }
    result[pair.slice(0, eq)] = pair.slice(eq + 1)
  }
  return result
}

export function randomNonce(byteLength = 16): string {
  const bytes = new Uint8Array(byteLength)
  crypto.getRandomValues(bytes)
  return Array.from(bytes)
    .map((byte) => byte.toString(16).padStart(2, '0'))
    .join('')
}
