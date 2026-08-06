export function parseHeaderText(text: string): Record<string, string> {
  const headers: Record<string, string> = {}

  for (const rawLine of text.split(/\r?\n/)) {
    const line = rawLine.trim().replace(/\s*\\$/, '').trim()
    if (line === '') {
      continue
    }

    const header = parseHeaderLine(line)
    if (!header) {
      continue
    }
    headers[header.key] = header.value
  }

  return headers
}

function parseHeaderLine(line: string): { key: string; value: string } | null {
  const candidate = extractCurlHeaderValue(line) ?? line
  const colonIndex = candidate.indexOf(':')
  if (colonIndex <= 0) {
    return null
  }

  const key = candidate.slice(0, colonIndex).trim()
  if (!isHeaderName(key)) {
    return null
  }

  return {
    key,
    value: candidate.slice(colonIndex + 1).trim(),
  }
}

function isHeaderName(value: string) {
  return /^[!#$%&'*+\-.^_`|~0-9A-Za-z]+$/.test(value)
}

function extractCurlHeaderValue(line: string) {
  const match = line.match(/(?:^|\s)(?:-H|--header)\s+(?:"([^"]*)"|'([^']*)'|(.+))$/)
  if (!match) {
    return null
  }
  return (match[1] ?? match[2] ?? match[3] ?? '').trim()
}
