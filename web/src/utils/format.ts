import type { TaskStatus } from '../api/client'

const ERROR_MESSAGE_LABELS: Record<string, string> = {
  'account not found': '账号不存在',
  'account group id is required': '账号组 ID 必填',
  'at least one account is required': '至少需要一个账号',
  'concurrency must be positive': '并发数必须大于 0',
  'concurrency must not exceed the number of accounts': '并发数不能超过账号数量',
  'group id is required': '账号组 ID 必填',
  'group_id is required': '账号组 ID 必填',
  'internal error': '服务内部错误',
  'interface name is required': '接口名称必填',
  'invalid json': '请求内容不是合法 JSON',
  'invalid load test config': '压测配置不合法',
  'invalid username or password': '用户名或密码错误',
  'invalid verification code': '验证码错误',
  'limit must be an integer': '条数限制必须是整数',
  'method is required': '请求方法必填',
  'module load_test scenario must contain at least one step': '场景至少需要一个步骤',
  'no results to export': '没有可导出的结果',
  'not found': '未找到',
  'password is required': '密码必填',
  'password must be at least 8 characters': '密码至少需要 8 个字符',
  'per account count must be positive': '每账号执行次数必须大于 0',
  'per_account_count must be positive': '每账号执行次数必须大于 0',
  'scenario must contain at least one step': '场景至少需要一个步骤',
  'scenario name is required': '场景名称必填',
  'scenario not found': '场景不存在',
  'task manager is shutting down': '服务正在停机，请稍后再试',
  'task not found': '任务不存在',
  'too many failed login attempts, try again later': '登录失败次数过多，请稍后再试',
  'total count must be positive': '总次数必须大于 0',
  'two-factor authentication is already enabled': '双因素认证已经启用',
  'two-factor authentication has not been set up for this account': '该账号尚未设置双因素认证',
  'two-factor verification code is required': '请输入双因素验证码',
  unauthorized: '未登录或登录已过期',
  'unknown module type': '未知模块类型',
  'username is required': '用户名必填',
  'username already exists': '用户名已存在',
  'url is required': 'URL 必填',
}

const STATUS_LABELS: Record<TaskStatus, string> = {
  pending: '等待中',
  running: '运行中',
  success: '成功',
  failed: '失败',
  canceled: '已取消',
}

const MODULE_TYPE_LABELS: Record<string, string> = {
  load_test: '接口压测',
}

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

export function formatTaskStatus(status: TaskStatus) {
  return STATUS_LABELS[status] ?? status
}

export function formatModuleType(moduleType: string | undefined) {
  if (!moduleType) {
    return '-'
  }
  return MODULE_TYPE_LABELS[moduleType] ?? moduleType
}

export function formatBoolean(value: boolean | undefined) {
  if (value === undefined) {
    return '-'
  }
  return value ? '是' : '否'
}

export function getErrorMessage(error: unknown) {
  if (error instanceof Error) {
    return ERROR_MESSAGE_LABELS[error.message] ?? error.message
  }
  return '请求失败'
}
