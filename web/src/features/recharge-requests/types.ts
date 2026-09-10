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
// 充值申请 + 多级审批类型定义,与后端 controller/recharge_request.go 对应。

export type RechargeCategory = 'project_delivery' | 'tech_research'

export type RechargeRequestStatus = 'pending' | 'approved' | 'rejected'

export type ApprovalDecision = 'pending' | 'approved' | 'rejected' | 'skipped'

export type RechargeLevelType = 'dept_leader' | 'designated'

export interface RechargeRequest {
  id: number
  user_id: number
  username: string
  category: RechargeCategory
  detail: string
  amount_usd: number
  quota: number
  status: RechargeRequestStatus
  current_level: number
  total_levels: number
  reject_reason: string
  create_time: number
  complete_time: number
}

export interface ApprovalStep {
  id: number
  request_id: number
  level: number
  user_id: number
  union_id: string
  name: string
  level_type: RechargeLevelType
  decision: ApprovalDecision
  comment: string
  decide_time: number
}

export interface ApprovalTask extends RechargeRequest {
  my_decision: string
  my_comment: string
  my_level: number
  is_actionable: boolean
}

export interface RechargeChainApprover {
  user_id: number
  name: string
  level_type: string
}

export interface RechargeChainLevel {
  level: number
  type: RechargeLevelType
  approvers: RechargeChainApprover[]
}

export interface RechargeRequestConfig {
  enabled: boolean
  quota_usd: number
  quota: number
  categories: RechargeCategory[]
  resolvable: boolean
  resolve_error?: string
  chain: RechargeChainLevel[]
}

export interface RechargeRequestListData {
  items: RechargeRequest[]
  total: number
}

export interface ApprovalTaskListData {
  items: ApprovalTask[]
  total: number
}

export interface RechargeRequestDetailData {
  request: RechargeRequest
  steps: ApprovalStep[]
}

export interface ApiResponse<T> {
  success: boolean
  message: string
  data: T
}
