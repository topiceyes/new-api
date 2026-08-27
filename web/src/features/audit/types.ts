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

/** 安全/行为审计事件(后端 model.AuditEvent)。 */
export interface AuditEvent {
  id: number
  created_at: number
  event_type: string
  severity: string
  user_id: number
  username: string
  token_id: number
  token_name: string
  channel_id: number
  model_name: string
  group: string
  ip: string
  user_agent: string
  request_id: string
  rule_id: string
  rule_name: string
  excerpt: string
  detail: string
  /** 仅详情接口返回;未按配置存储时为空串 */
  prompt: string
  /** LLM 分类结果(二期②),未分类时为空串 */
  category: string
}

/** LLM 分类沉淀的 skill 候选(待管理员审核)。 */
export interface SkillCandidate {
  id: number
  created_at: number
  updated_at: number
  title: string
  category: string
  sample_prompt: string
  occurrence_count: number
  user_count: number
  status: string
  published_skill_id: number
}

/** 已发布/已下架的 skill 库条目。 */
export interface Skill {
  id: number
  created_at: number
  updated_at: number
  title: string
  category: string
  description: string
  sample_prompt: string
  status: string
}

export interface AuditEventListData {
  items: AuditEvent[]
  total: number
}

export interface AuditEventStatRow {
  event_type: string
  rule_id: string
  count: number
}

export interface AuditEventFilters {
  event_type?: string
  severity?: string
  keyword?: string
  start_timestamp?: number
  end_timestamp?: number
}

export interface ApiResponse<T> {
  success: boolean
  message: string
  data: T
}

export const AUDIT_EVENT_TYPES = [
  'pii_hit',
  'key_share_multi_ip',
  'key_share_rapid_ip',
  'key_share_multi_ua',
  'key_share_impossible_travel',
  'prompt_snapshot',
  'response_malicious',
] as const

export const AUDIT_SEVERITIES = ['info', 'warning', 'critical'] as const
