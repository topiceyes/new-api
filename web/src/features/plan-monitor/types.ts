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

// 套餐监控(上游 token plan 用量)类型定义,与后端 controller/plan_monitor.go 对应。

export interface PlanMonitorUsageView {
  period: string // '5h' | 'weekly' | 'monthly'
  used_percent: number
  remaining_percent: number
  period_end_time: number // 周期截止时间戳(秒)
  fetched_at: number
}

export interface PlanMonitorPlan {
  id: number
  provider: string
  plan_name: string
  api_url: string
  api_key_masked: string
  refresh_interval_min: number
  sort_order: number // 排序权重,越小越靠前
  enabled: boolean
  created_time: number
  updated_time: number
  last_fetch_time: number
  last_error: string
}

export interface PlanMonitorOverviewItem extends PlanMonitorPlan {
  usages: PlanMonitorUsageView[]
}

export interface PlanMonitorOverviewGroup {
  provider: string
  plans: PlanMonitorOverviewItem[]
}

export interface PlanMonitorPayload {
  provider: string
  plan_name: string
  api_url: string
  api_key: string // 编辑时留空表示不修改
  refresh_interval_min: number
  sort_order: number
  enabled: boolean
}

export interface PlanMonitorListData {
  plans: PlanMonitorPlan[]
  supported_providers: string[]
}

export interface PlanMonitorOverviewData {
  groups: PlanMonitorOverviewGroup[]
}

export interface ApiResponse<T> {
  success: boolean
  message: string
  data: T
}
