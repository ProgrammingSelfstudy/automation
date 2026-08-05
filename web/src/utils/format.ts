export function formatDateTime(value: string | undefined) {
  if (!value) {
    return '-'
  }
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return value
  }
  return new Intl.DateTimeFormat('zh-CN', {
    dateStyle: 'short',
    timeStyle: 'medium',
  }).format(date)
}

export function formatNumber(value: number | undefined) {
  return new Intl.NumberFormat('zh-CN').format(value ?? 0)
}

export function getErrorMessage(error: unknown) {
  if (error instanceof Error) {
    return error.message
  }
  return '请求失败'
}
