// Pure-JS MD5 (RFC 1321). Used client-side only, so a legacy device-signature
// secret never has to leave the browser to compute a request signature.

const SHIFTS = [
  7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22, 5, 9, 14, 20, 5, 9, 14, 20, 5, 9, 14,
  20, 5, 9, 14, 20, 4, 11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23, 6, 10, 15, 21, 6,
  10, 15, 21, 6, 10, 15, 21, 6, 10, 15, 21,
]

const K = new Int32Array([
  -680876936, -389564586, 606105819, -1044525330, -176418897, 1200080426, -1473231341, -45705983,
  1770035416, -1958414417, -42063, -1990404162, 1804603682, -40341101, -1502002290, 1236535329,
  -165796510, -1069501632, 643717713, -373897302, -701558691, 38016083, -660478335, -405537848,
  568446438, -1019803690, -187363961, 1163531501, -1444681467, -51403784, 1735328473, -1926607734,
  -378558, -2022574463, 1839030562, -35309556, -1530992060, 1272893353, -155497632, -1094730640,
  681279174, -358537222, -722521979, 76029189, -640364487, -421815835, 530742520, -995338651,
  -198630844, 1126891415, -1416354905, -57434055, 1700485571, -1894986606, -1051523, -2054922799,
  1873313359, -30611744, -1560198380, 1309151649, -145523070, -1120210379, 718787259, -343485551,
])

function rotl(x: number, bits: number) {
  return (x << bits) | (x >>> (32 - bits))
}

function toHexLE(word: number) {
  const bytes = new Uint8Array(4)
  new DataView(bytes.buffer).setInt32(0, word, true)
  return Array.from(bytes)
    .map((byte) => byte.toString(16).padStart(2, '0'))
    .join('')
}

// md5 returns the lowercase hex digest of message (UTF-8 encoded).
export function md5(message: string): string {
  const input = new TextEncoder().encode(message)
  const bitLength = input.length * 8

  const paddingLength = input.length % 64 < 56 ? 56 - (input.length % 64) : 120 - (input.length % 64)
  const total = input.length + paddingLength + 8
  const buffer = new Uint8Array(total)
  buffer.set(input, 0)
  buffer[input.length] = 0x80

  const view = new DataView(buffer.buffer)
  view.setUint32(total - 8, bitLength >>> 0, true)
  view.setUint32(total - 4, Math.floor(bitLength / 4294967296), true)

  let a0 = 0x67452301
  let b0 = 0xefcdab89
  let c0 = 0x98badcfe
  let d0 = 0x10325476

  for (let chunkStart = 0; chunkStart < total; chunkStart += 64) {
    const chunk = new Int32Array(16)
    for (let word = 0; word < 16; word++) {
      chunk[word] = view.getInt32(chunkStart + word * 4, true)
    }

    let a = a0
    let b = b0
    let c = c0
    let d = d0

    for (let i = 0; i < 64; i++) {
      let f: number
      let g: number
      if (i < 16) {
        f = (b & c) | (~b & d)
        g = i
      } else if (i < 32) {
        f = (d & b) | (~d & c)
        g = (5 * i + 1) % 16
      } else if (i < 48) {
        f = b ^ c ^ d
        g = (3 * i + 5) % 16
      } else {
        f = c ^ (b | ~d)
        g = (7 * i) % 16
      }
      f = (f + a + K[i] + chunk[g]) | 0
      a = d
      d = c
      c = b
      b = (b + rotl(f, SHIFTS[i])) | 0
    }

    a0 = (a0 + a) | 0
    b0 = (b0 + b) | 0
    c0 = (c0 + c) | 0
    d0 = (d0 + d) | 0
  }

  return toHexLE(a0) + toHexLE(b0) + toHexLE(c0) + toHexLE(d0)
}
