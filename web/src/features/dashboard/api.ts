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
  AnalyticsAccess,
  AnalyticsActivity,
  AnalyticsDepartment,
  AnalyticsHeatmapCell,
  AnalyticsModel,
  AnalyticsOverview,
  AnalyticsTopUser,
  FlowQuotaDataItem,
  QuotaDataItem,
  UptimeGroupResult,
} from './types'

// ============================================================================
// Dashboard APIs
// ============================================================================

// ----------------------------------------------------------------------------
// Quota & Usage Data
// ----------------------------------------------------------------------------

// Get user quota data within a time range
// Admin users get all users' data by default.
export async function getUserQuotaDates(
  params: {
    start_timestamp: number
    end_timestamp: number
    default_time?: string
    username?: string
  },
  isAdmin = false
) {
  const endpoint = isAdmin ? '/api/data' : '/api/data/self'
  const res = await api.get<{ success: boolean; data: QuotaDataItem[] }>(
    endpoint,
    { params }
  )
  return res.data
}

// ----------------------------------------------------------------------------
// System Monitoring
// ----------------------------------------------------------------------------

export async function getUserQuotaDataByUsers(params: {
  start_timestamp: number
  end_timestamp: number
}) {
  const res = await api.get<{ success: boolean; data: QuotaDataItem[] }>(
    '/api/data/users',
    { params }
  )
  return res.data
}

export async function getFlowQuotaDates(
  params: {
    start_timestamp: number
    end_timestamp: number
    default_time?: string
    username?: string
  },
  isAdmin = false
) {
  const endpoint = isAdmin ? '/api/data/flow' : '/api/data/flow/self'
  const res = await api.get<{
    success: boolean
    data?: FlowQuotaDataItem[]
    message?: string
  }>(endpoint, { params })
  return res.data
}

// Get uptime monitoring status for all services
export async function getUptimeStatus() {
  const res = await api.get<{ success: boolean; data: UptimeGroupResult[] }>(
    '/api/uptime/status'
  )
  return res.data
}

// ----------------------------------------------------------------------------
// Usage Analytics (/api/analytics/*)
// ----------------------------------------------------------------------------

interface AnalyticsRangeParams {
  start_timestamp: number
  end_timestamp: number
}

export async function getAnalyticsAccess() {
  const res = await api.get<{ success: boolean; data: AnalyticsAccess }>(
    '/api/analytics/access'
  )
  return res.data
}

export async function getAnalyticsOverview(params: AnalyticsRangeParams) {
  const res = await api.get<{ success: boolean; data: AnalyticsOverview }>(
    '/api/analytics/overview',
    { params }
  )
  return res.data
}

export async function getAnalyticsActivity(params: AnalyticsRangeParams) {
  const res = await api.get<{ success: boolean; data: AnalyticsActivity }>(
    '/api/analytics/activity',
    { params }
  )
  return res.data
}

export async function getAnalyticsTopUsers(
  params: AnalyticsRangeParams & { limit?: number }
) {
  const res = await api.get<{ success: boolean; data: AnalyticsTopUser[] }>(
    '/api/analytics/top-users',
    { params }
  )
  return res.data
}

export async function getAnalyticsDepartments(params: AnalyticsRangeParams) {
  const res = await api.get<{
    success: boolean
    data: AnalyticsDepartment[]
  }>('/api/analytics/departments', { params })
  return res.data
}

export async function getAnalyticsModels(params: AnalyticsRangeParams) {
  const res = await api.get<{ success: boolean; data: AnalyticsModel[] }>(
    '/api/analytics/models',
    { params }
  )
  return res.data
}

export async function getAnalyticsHeatmap(params: AnalyticsRangeParams) {
  const res = await api.get<{
    success: boolean
    data: { cells: AnalyticsHeatmapCell[] }
  }>('/api/analytics/heatmap', { params })
  return res.data
}
