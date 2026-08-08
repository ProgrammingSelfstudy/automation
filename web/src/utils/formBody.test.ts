import assert from 'node:assert/strict'
import test from 'node:test'

import { decodeFormBody, encodeFormBody } from './formBody.ts'

test('encodeFormBody joins pairs with & and encodes special characters', () => {
  assert.equal(
    encodeFormBody({ grant_type: 'email', email: 'test@example.com', note: 'a b&c' }),
    'grant_type=email&email=test%40example.com&note=a+b%26c',
  )
})

test('encodeFormBody preserves template placeholders unencoded', () => {
  assert.equal(
    encodeFormBody({ email: '{{.account.username}}', password: '{{.account.password}}' }),
    'email={{.account.username}}&password={{.account.password}}',
  )
})

test('decodeFormBody decodes percent-encoding and + as space', () => {
  assert.deepEqual(decodeFormBody('grant_type=email&email=test%40example.com&note=a+b%26c'), {
    grant_type: 'email',
    email: 'test@example.com',
    note: 'a b&c',
  })
})

test('decodeFormBody leaves template placeholders intact', () => {
  assert.deepEqual(decodeFormBody('email={{.account.username}}&password={{.account.password}}'), {
    email: '{{.account.username}}',
    password: '{{.account.password}}',
  })
})

test('decodeFormBody handles empty string and trailing/leading &', () => {
  assert.deepEqual(decodeFormBody(''), {})
  assert.deepEqual(decodeFormBody('&a=1&&b=2&'), { a: '1', b: '2' })
})

test('round trip through encode then decode recovers the original pairs', () => {
  const original = {
    grant_type: 'email',
    email: '{{.account.username}}',
    password: '{{.account.password}}',
    note: 'a b&c=d',
  }
  assert.deepEqual(decodeFormBody(encodeFormBody(original)), original)
})
