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
*/
import { api } from '@/lib/api'

import type {
  ApiResponse,
  AuditEvent,
  AuditEventFilters,
  AuditEventListData,
  AuditEventStatRow,
  Skill,
  SkillCandidate,
} from './types'

const BASE = '/api/audit'

export async function listAuditEvents(
  page: number,
  pageSize: number,
  filters: AuditEventFilters
): Promise<ApiResponse<AuditEventListData>> {
  const res = await api.get(`${BASE}/events`, {
    params: {
      p: page,
      page_size: pageSize,
      ...filters,
    },
  })
  return res.data
}

export async function getAuditEvent(
  id: number
): Promise<ApiResponse<AuditEvent>> {
  const res = await api.get(`${BASE}/events/${id}`)
  return res.data
}

export async function getAuditEventStats(
  startTimestamp?: number,
  endTimestamp?: number
): Promise<ApiResponse<AuditEventStatRow[]>> {
  const res = await api.get(`${BASE}/stats`, {
    params: {
      start_timestamp: startTimestamp,
      end_timestamp: endTimestamp,
    },
  })
  return res.data
}

export async function listSkillCandidates(
  page: number,
  pageSize: number,
  status: string,
  keyword: string
): Promise<ApiResponse<{ items: SkillCandidate[]; total: number }>> {
  const res = await api.get(`${BASE}/skills/candidates`, {
    params: { p: page, page_size: pageSize, status, keyword },
  })
  return res.data
}

export async function approveSkillCandidate(
  id: number,
  data: { title?: string; category?: string; description?: string }
): Promise<ApiResponse<Skill>> {
  const res = await api.post(`${BASE}/skills/candidates/${id}/approve`, data)
  return res.data
}

export async function rejectSkillCandidate(
  id: number
): Promise<ApiResponse<null>> {
  const res = await api.post(`${BASE}/skills/candidates/${id}/reject`)
  return res.data
}

export async function listSkills(
  page: number,
  pageSize: number,
  status: string,
  keyword: string
): Promise<ApiResponse<{ items: Skill[]; total: number }>> {
  const res = await api.get(`${BASE}/skills`, {
    params: { p: page, page_size: pageSize, status, keyword },
  })
  return res.data
}

export async function updateSkill(
  id: number,
  data: {
    title: string
    category?: string
    description?: string
    sample_prompt?: string
  }
): Promise<ApiResponse<null>> {
  const res = await api.put(`${BASE}/skills/${id}`, data)
  return res.data
}

export async function archiveSkill(id: number): Promise<ApiResponse<null>> {
  const res = await api.post(`${BASE}/skills/${id}/archive`)
  return res.data
}
