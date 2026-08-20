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
import { api } from '@/lib/api'

import type {
  ApiResponse,
  PlanMonitorListData,
  PlanMonitorOverviewData,
  PlanMonitorPayload,
  PlanMonitorPlan,
} from './types'

const BASE = '/api/plan_monitor/admin'

export async function listPlans(): Promise<ApiResponse<PlanMonitorListData>> {
  const res = await api.get(`${BASE}/plans`)
  return res.data
}

export async function createPlan(
  data: PlanMonitorPayload
): Promise<ApiResponse<{ plan: PlanMonitorPlan }>> {
  const res = await api.post(`${BASE}/plans`, data)
  return res.data
}

export async function updatePlan(
  id: number,
  data: PlanMonitorPayload
): Promise<ApiResponse<{ plan: PlanMonitorPlan }>> {
  const res = await api.put(`${BASE}/plans/${id}`, data)
  return res.data
}

export async function deletePlan(id: number): Promise<ApiResponse<null>> {
  const res = await api.delete(`${BASE}/plans/${id}`)
  return res.data
}

export async function setPlanStatus(
  id: number,
  enabled: boolean
): Promise<ApiResponse<null>> {
  const res = await api.patch(`${BASE}/plans/${id}/status`, { enabled })
  return res.data
}

export async function refreshPlan(id: number): Promise<ApiResponse<null>> {
  const res = await api.post(`${BASE}/plans/${id}/refresh`)
  return res.data
}

export async function getOverview(): Promise<
  ApiResponse<PlanMonitorOverviewData>
> {
  const res = await api.get(`${BASE}/overview`)
  return res.data
}
