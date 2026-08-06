import assert from 'node:assert/strict'
import test from 'node:test'

import { parseHeaderText } from './headers.ts'

test('parseHeaderText parses plain header lines', () => {
  assert.deepEqual(
    parseHeaderText(`
Content-Type: application/json
Authorization: Bearer xxx
`),
    {
      'Content-Type': 'application/json',
      Authorization: 'Bearer xxx',
    },
  )
})

test('parseHeaderText parses curl -H header lines', () => {
  assert.deepEqual(
    parseHeaderText(`
-H "X-Custom: 1"
--header 'X-Token: abc:def'
curl -H "X-Curl: yes" \\
`),
    {
      'X-Custom': '1',
      'X-Token': 'abc:def',
      'X-Curl': 'yes',
    },
  )
})

test('parseHeaderText skips invalid lines and keeps the last duplicate', () => {
  assert.deepEqual(
    parseHeaderText(`
Content-Type: text/plain
not a header
Bad Header: no
: missing-key
Content-Type: application/json
`),
    {
      'Content-Type': 'application/json',
    },
  )
})
