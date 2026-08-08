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

export function randomNonce(byteLength = 16): string {
  const bytes = new Uint8Array(byteLength)
  crypto.getRandomValues(bytes)
  return Array.from(bytes)
    .map((byte) => byte.toString(16).padStart(2, '0'))
    .join('')
}
