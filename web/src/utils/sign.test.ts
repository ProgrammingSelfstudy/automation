import assert from 'node:assert/strict'
import test from 'node:test'

import { buildSignParams, computeAppServerSign, parseFormBody } from './sign.ts'

test('buildSignParams sorts by key and joins without encoding', () => {
  assert.equal(
    buildSignParams({ email: 'a@b.com', grant_type: 'email', password: 'abc' }),
    'email=a@b.com&grant_type=email&password=abc',
  )
})

test('buildSignParams skips empty values, null, and the literal string "null"', () => {
  assert.equal(buildSignParams({ b: '', a: 'x', c: 'null', d: 'y' }), 'a=x&d=y')
})

test('buildSignParams returns empty string for no usable params', () => {
  assert.equal(buildSignParams({}), '')
  assert.equal(buildSignParams({ a: '', b: 'null' }), '')
})

test('computeAppServerSign matches an independently computed MD5', () => {
  // Cross-checked against:
  // python3 -c "import hashlib; print(hashlib.md5(('email=a@b.com&grant_type=email' + 'nonce123' + 'ts456' + 'secretXYZ').encode()).hexdigest().upper())"
  assert.equal(
    computeAppServerSign({ email: 'a@b.com', grant_type: 'email' }, 'nonce123', 'ts456', 'secretXYZ'),
    'C110A320F4F5550675FBE199477334F6',
  )
})

test('parseFormBody splits on & and the first =', () => {
  assert.deepEqual(parseFormBody('email=a%40b.com&grant_type=email&password='), {
    email: 'a%40b.com',
    grant_type: 'email',
    password: '',
  })
})

test('parseFormBody ignores empty segments', () => {
  assert.deepEqual(parseFormBody(''), {})
  assert.deepEqual(parseFormBody('a=1&&b=2'), { a: '1', b: '2' })
})
