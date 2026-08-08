import assert from 'node:assert/strict'
import test from 'node:test'

import { decodeFormBody } from './formBody.ts'
import { buildSignParams, computeAppServerSign } from './sign.ts'

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

// decodeFormBody 必须在 computeAppServerSign 之前跑，不能直接对 body_tpl
// 按 & / = 裸拆分：2026-08-08 debug 一个真实接口的 403 时确认了，body 里
// 的 @ 被 form-urlencoded 编码成 %40，对编码后的值（email=a%40b.com）算出
// 来的签名服务端校验必定失败——服务端签名用的是解码后的值。这条测试值是
// 拿这次调试用的假密钥（不是真实密钥，真实密钥不进仓库）验证 decodeFormBody
// 到 computeAppServerSign 这条链路本身接得对，不是在验证某个真实接口。
test('computeAppServerSign on a decoded form body differs from signing the raw encoded body', () => {
  const rawBody = 'grant_type=email&email=a%40b.com&password=abc'
  const decoded = decodeFormBody(rawBody)
  assert.deepEqual(decoded, { grant_type: 'email', email: 'a@b.com', password: 'abc' })

  const signOnDecoded = computeAppServerSign(decoded, 'nonce123', 'ts456', 'secretXYZ')
  // 用同样的输入手算一遍交叉验证，不是自己骗自己：
  // python3 -c "import hashlib; print(hashlib.md5(('email=a@b.com&grant_type=email&password=abc' + 'nonce123' + 'ts456' + 'secretXYZ').encode()).hexdigest().upper())"
  assert.equal(signOnDecoded, '3349CEC03E60FA8208E7828C3D9D926E')

  // 对没解码的原始 body 硬拆分（旧实现的错误做法）会算出不一样的签名——
  // 这就是当年那个 403 的根因，这里断言"两者不相等"防止以后有人图省事
  // 又把 decodeFormBody 换回裸拆分。
  const rawSplit = Object.fromEntries(rawBody.split('&').map((pair) => pair.split('=') as [string, string]))
  const signOnRaw = computeAppServerSign(rawSplit, 'nonce123', 'ts456', 'secretXYZ')
  assert.notEqual(signOnRaw, signOnDecoded)
})
