// application/x-www-form-urlencoded 请求体的键值对编辑器（StepFields.tsx）
// 用这两个函数在"原始字符串"（body_tpl，真正发给后端的东西）和"键值对"
// （KeyValueEditor 要的 Record<string,string>）之间互转。
//
// body_tpl 在发送前会先过一遍 Go 的 text/template 渲染（{{.account.username}}
// 这种账号池变量占位符），发送的是渲染后的结果，不是模板本身——所以这里的
// 编码/解码只能处理占位符*外面*的部分，占位符内部的 {{ }} 原样保留，不能被
// percent-encode 成 %7B%7B，不然模板引擎在 body_tpl 里根本认不出这是个占位符。
const PLACEHOLDER_RE = /(\{\{.*?\}\})/g

function encodeFormComponent(raw: string): string {
  return raw
    .split(PLACEHOLDER_RE)
    .map((part) => (part.startsWith('{{') && part.endsWith('}}') ? part : encodeURIComponent(part).replace(/%20/g, '+')))
    .join('')
}

export function encodeFormBody(pairs: Record<string, string>): string {
  return Object.entries(pairs)
    .map(([key, value]) => `${encodeFormComponent(key)}=${encodeFormComponent(value)}`)
    .join('&')
}

// 跟 utils/sign.ts 的 parseFormBody 不是同一个东西，故意换了名字避免混淆：
// sign.ts 那个是给签名计算用的，故意不解码（要保留请求实际发出去的原始
// 字节）；这个是给键值对编辑器用的，要解码成人能读的值再显示出来。
//
// decodeURIComponent 只处理 %XX 转义序列，原样保留没有被转义过的字符（包括
// 模板占位符里的 {{ }}），所以解析方向不用像编码那样特殊处理占位符。
export function decodeFormBody(raw: string): Record<string, string> {
  const result: Record<string, string> = {}
  const trimmed = raw.trim()
  if (trimmed === '') {
    return result
  }

  for (const pair of trimmed.split('&')) {
    if (pair === '') {
      continue
    }
    const eqIndex = pair.indexOf('=')
    const rawKey = eqIndex === -1 ? pair : pair.slice(0, eqIndex)
    const rawValue = eqIndex === -1 ? '' : pair.slice(eqIndex + 1)
    const key = safeDecode(rawKey)
    if (key === '') {
      continue
    }
    result[key] = safeDecode(rawValue)
  }

  return result
}

function safeDecode(value: string): string {
  try {
    return decodeURIComponent(value.replace(/\+/g, ' '))
  } catch {
    // 不是合法的 percent-encoding（比如本来就是模板占位符或者用户手滑打错），
    // 原样返回比整个解析报错更有用。
    return value
  }
}
