/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

// 用量展示纯函数,供组件与测试共用。

export type UsageLevel = 'green' | 'yellow' | 'red'

// 进度条配色阈值:<80 绿,80–90 黄,>90 红(90 整归红档)。
export function usageLevel(usedPercent: number): UsageLevel {
  if (usedPercent >= 90) return 'red'
  if (usedPercent >= 80) return 'yellow'
  return 'green'
}

// 周期标识 → 展示文案 key
export const PERIOD_LABEL_KEYS: Record<string, string> = {
  '5h': 'Every 5 hours',
  weekly: 'Weekly',
  monthly: 'Monthly',
}

export function periodLabelKey(period: string): string {
  return PERIOD_LABEL_KEYS[period] ?? period
}

// 秒时间戳 → 本地可读时间;0/无效返回 '-'。
export function formatPeriodEnd(sec: number): string {
  if (!sec || sec <= 0) return '-'
  const d = new Date(sec * 1000)
  if (Number.isNaN(d.getTime())) return '-'
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

// 供应商标识 → 展示名
export const PROVIDER_LABELS: Record<string, string> = {
  minimax: 'MiniMax',
  kimi: 'Kimi',
  bigmodel: 'BigModel Personal (智谱个人版)',
  bigmodel_enterprise: 'BigModel Enterprise (智谱企业版)',
  volcengine: 'Volcengine Ark Agent Plan (火山方舟Agent)',
  volcengine_coding: 'Volcengine Ark Coding Plan (火山方舟Coding)',
  opencode: 'OpenCode Go (opencode.ai)',
}

export function providerLabel(provider: string): string {
  return PROVIDER_LABELS[provider] ?? provider
}
