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
export interface ApiResponse<T> {
  success: boolean
  message: string
  data: T
}

export interface OrgDepartment {
  id: number
  provider: string
  dept_id: string
  parent_id: string
  name: string
  leader_user_ids: string
  member_count: number
  sort_order: number
  synced_at: number
}

export interface OrgMember {
  id: number
  provider: string
  union_id: string
  provider_user_id: string
  name: string
  title: string
  /** JSON 数组字符串:所属部门 id 列表 */
  dept_ids: string
  /** JSON 数组字符串:在哪些部门是主管 */
  leader_dept_ids: string
  /** 匹配到的本地用户 id,0=未绑定 */
  user_id: number
  username?: string
  display_name?: string
  group_mapped: boolean
  synced_at: number
}

export interface OrgOverviewData {
  provider: string
  departments: OrgDepartment[]
  members: OrgMember[]
  synced_at: number
}

export interface OrgSyncTask {
  task_id: string
  type: string
  status: 'pending' | 'running' | 'succeeded' | 'failed'
  error: string
  result?: {
    provider?: string
    departments?: number
    members?: number
    matched?: number
    group_mapped?: number
    group_unmapped?: number
  } | null
  created_at: number
  updated_at: number
}

/** 解析 JSON 数组字符串字段,解析失败按空数组处理(后端保证合法,这里只是防御) */
export function parseIdList(raw: string): string[] {
  if (!raw) return []
  try {
    const parsed = JSON.parse(raw)
    return Array.isArray(parsed) ? parsed.map(String) : []
  } catch {
    return []
  }
}
