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
import { z } from 'zod'

import {
  type PermissionCatalog,
  type AdminPermissionMatrix,
  normalizeAdminPermissions,
} from '@/lib/admin-permissions'
import { quotaUnitsToDollars } from '@/lib/format'
import { ROLE } from '@/lib/roles'

import { DEFAULT_GROUP } from '../constants'
import { type UserFormData, type User } from '../types'

// ============================================================================
// Form Schema
// ============================================================================

export const userFormSchema = z.object({
  username: z.string().min(1, 'Username is required'),
  display_name: z.string().optional(),
  password: z.string().optional(),
  role: z.number().optional(),
  quota_dollars: z.number().min(0).optional(),
  group: z.string().optional(),
  remark: z.string().optional(),
  // Admin-managed per-user API rate limit; 0 = unlimited. Lives in the user's
  // setting JSON, not a top-level column.
  rate_limit_rpm: z.number().int().min(0).optional(),
  // Raw setting JSON of the row being edited, kept so the RPM value can be
  // merged without dropping unrelated keys.
  setting: z.string().optional(),
  admin_permissions: z
    .record(z.string(), z.record(z.string(), z.boolean()))
    .optional(),
})

export type UserFormValues = z.infer<typeof userFormSchema>

// ============================================================================
// Form Defaults
// ============================================================================

export const USER_FORM_DEFAULT_VALUES: UserFormValues = {
  username: '',
  display_name: '',
  password: '',
  role: 1, // Default to common user
  quota_dollars: 0,
  group: DEFAULT_GROUP,
  remark: '',
  rate_limit_rpm: 0,
  setting: '',
  // Filled against the backend catalog at render time; see UsersMutateDrawer.
  admin_permissions: {},
}

// ============================================================================
// Form Data Transformation
// ============================================================================

/**
 * Merge the admin-configured RPM into the user's setting JSON. Returns
 * undefined when nothing should be sent (no change needed, or the existing
 * setting is unparseable — in that case we leave the server value untouched
 * rather than risk dropping the user's other settings).
 */
function mergeRateLimitRpmSetting(
  raw: string | undefined,
  rpm: number
): string | undefined {
  let obj: Record<string, unknown> = {}
  if (raw) {
    try {
      obj = JSON.parse(raw)
    } catch {
      return undefined
    }
  }
  const prev = typeof obj.rate_limit_rpm === 'number' ? obj.rate_limit_rpm : 0
  if (prev === rpm) {
    return undefined
  }
  if (rpm > 0) {
    obj.rate_limit_rpm = rpm
  } else {
    delete obj.rate_limit_rpm
  }
  return JSON.stringify(obj)
}

/**
 * Transform form data to API payload
 */
export function transformFormDataToPayload(
  data: UserFormValues,
  userId?: number,
  catalog?: PermissionCatalog
): UserFormData & { id?: number } {
  const payload: UserFormData & { id?: number } = {
    username: data.username,
    display_name: data.display_name || data.username,
    password: data.password || undefined,
  }

  const role = userId === undefined ? data.role || 1 : (data.role ?? 0)

  // Only send the permission matrix when the target is an admin and the catalog
  // is available; without the catalog we cannot build a full matrix, so we omit
  // the field (the backend then leaves existing permissions untouched).
  if (role >= ROLE.ADMIN && catalog) {
    payload.admin_permissions = normalizeAdminPermissions(
      data.admin_permissions as AdminPermissionMatrix | undefined,
      catalog
    )
  }

  // For create: only send required fields
  if (userId === undefined) {
    payload.role = role
    if (data.rate_limit_rpm && data.rate_limit_rpm > 0) {
      payload.setting = JSON.stringify({ rate_limit_rpm: data.rate_limit_rpm })
    }
  } else {
    // For update: quota is adjusted atomically via /api/user/manage, not sent here
    payload.group = data.group
    payload.remark = data.remark || undefined
    payload.id = userId
    const setting = mergeRateLimitRpmSetting(data.setting, data.rate_limit_rpm ?? 0)
    if (setting !== undefined) {
      payload.setting = setting
    }
  }

  return payload
}

/**
 * Transform user data to form defaults. The admin permission matrix is passed
 * through as-is (the backend already returns a full matrix); it is filled against
 * the catalog at render time in UsersMutateDrawer.
 */
export function transformUserToFormDefaults(user: User): UserFormValues {
  let rateLimitRPM = 0
  if (user.setting) {
    try {
      const parsed = JSON.parse(user.setting) as Record<string, unknown>
      if (typeof parsed.rate_limit_rpm === 'number' && parsed.rate_limit_rpm > 0) {
        rateLimitRPM = parsed.rate_limit_rpm
      }
    } catch {
      // Unparseable setting JSON: leave RPM at 0 and keep the raw string so a
      // save without RPM changes does not rewrite it.
    }
  }
  return {
    username: user.username,
    display_name: user.display_name,
    password: '',
    role: user.role,
    quota_dollars: quotaUnitsToDollars(user.quota),
    group: user.group || DEFAULT_GROUP,
    remark: user.remark || '',
    rate_limit_rpm: rateLimitRPM,
    setting: user.setting || '',
    admin_permissions: user.admin_permissions ?? {},
  }
}
