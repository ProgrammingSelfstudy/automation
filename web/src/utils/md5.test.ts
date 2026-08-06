import assert from 'node:assert/strict'
import test from 'node:test'

import { md5 } from './md5.ts'

// RFC 1321 section A.5 test suite.
test('md5 matches RFC 1321 test vectors', () => {
  assert.equal(md5(''), 'd41d8cd98f00b204e9800998ecf8427e')
  assert.equal(md5('a'), '0cc175b9c0f1b6a831c399e269772661')
  assert.equal(md5('abc'), '900150983cd24fb0d6963f7d28e17f72')
  assert.equal(md5('message digest'), 'f96b697d7cb7938d525a2f31aaf161d0')
  assert.equal(md5('abcdefghijklmnopqrstuvwxyz'), 'c3fcd3d76192e4007dfb496cca67e13b')
  assert.equal(
    md5('ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789'),
    'd174ab98d277d9f5a5611c2c9f419d9f',
  )
  assert.equal(
    md5('12345678901234567890123456789012345678901234567890123456789012345678901234567890'),
    '57edf4a22be3c955ac49da2e2107b67a',
  )
})

test('md5 handles multi-byte UTF-8 input', () => {
  // Cross-checked against `python3 -c "import hashlib; print(hashlib.md5('接口压测'.encode()).hexdigest())"`.
  assert.equal(md5('接口压测'), 'a48ac534494611359ad13d147c8794a2')
})
