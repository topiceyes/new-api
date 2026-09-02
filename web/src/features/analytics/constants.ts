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
import type { TFunction } from 'i18next'

import type { StatusVariant } from '@/components/status-badge'

import type { AnalyticsUserStatus } from './types'

export const ANALYTICS_USER_STATUS = {
  ACTIVE: 'active',
  SILENT: 'silent',
  NEVER: 'never',
} as const

export const ANALYTICS_USER_STATUS_CONFIG: Record<
  AnalyticsUserStatus,
  { labelKey: string; variant: StatusVariant }
> = {
  active: { labelKey: 'Active', variant: 'success' },
  silent: { labelKey: 'Silent', variant: 'warning' },
  never: { labelKey: 'Never Used', variant: 'neutral' },
}

// Sort rank matches the backend default ordering (active first).
export function analyticsUserStatusRank(status: AnalyticsUserStatus): number {
  switch (status) {
    case 'active':
      return 0
    case 'silent':
      return 1
    default:
      return 2
  }
}

export function getAnalyticsUserStatusOptions(t: TFunction) {
  return (Object.keys(ANALYTICS_USER_STATUS_CONFIG) as AnalyticsUserStatus[]).map(
    (value) => ({
      value,
      label: t(ANALYTICS_USER_STATUS_CONFIG[value].labelKey),
    })
  )
}
