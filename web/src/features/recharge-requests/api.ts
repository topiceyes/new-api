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
  ApprovalTaskListData,
  RechargeCategory,
  RechargeRequestConfig,
  RechargeRequestDetailData,
  RechargeRequestListData,
  RechargeRequestStatus,
} from './types'

const BASE = '/api/recharge_request'

export async function getRechargeConfig(): Promise<
  ApiResponse<RechargeRequestConfig>
> {
  const res = await api.get(`${BASE}/config`)
  return res.data
}

export async function submitRechargeRequest(data: {
  category: RechargeCategory
  detail: string
}): Promise<ApiResponse<number>> {
  const res = await api.post(`${BASE}/`, data)
  return res.data
}

export async function listMyRechargeRequests(
  page: number,
  pageSize: number
): Promise<ApiResponse<RechargeRequestListData>> {
  const res = await api.get(`${BASE}/self`, {
    params: { p: page, page_size: pageSize },
  })
  return res.data
}

export async function listMyApprovalTasks(
  pending: boolean,
  page: number,
  pageSize: number
): Promise<ApiResponse<ApprovalTaskListData>> {
  const res = await api.get(`${BASE}/approvals`, {
    params: { pending, p: page, page_size: pageSize },
  })
  return res.data
}

export async function getRechargeRequestDetail(
  id: number,
  admin = false
): Promise<ApiResponse<RechargeRequestDetailData>> {
  const url = admin ? `${BASE}/admin/requests/${id}` : `${BASE}/${id}`
  const res = await api.get(url)
  return res.data
}

export async function approveRechargeRequest(
  id: number,
  comment?: string
): Promise<ApiResponse<unknown>> {
  const res = await api.post(`${BASE}/${id}/approve`, { comment })
  return res.data
}

export async function rejectRechargeRequest(
  id: number,
  comment: string
): Promise<ApiResponse<unknown>> {
  const res = await api.post(`${BASE}/${id}/reject`, { comment })
  return res.data
}

export interface AdminRechargeRequestParams {
  status?: RechargeRequestStatus | ''
  category?: RechargeCategory | ''
  keyword?: string
  page: number
  pageSize: number
}

export async function listAdminRechargeRequests(
  params: AdminRechargeRequestParams
): Promise<ApiResponse<RechargeRequestListData>> {
  const res = await api.get(`${BASE}/admin/requests`, {
    params: {
      status: params.status || undefined,
      category: params.category || undefined,
      keyword: params.keyword || undefined,
      p: params.page,
      page_size: params.pageSize,
    },
  })
  return res.data
}
