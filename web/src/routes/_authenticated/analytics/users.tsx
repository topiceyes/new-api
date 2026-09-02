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
import { createFileRoute, redirect } from '@tanstack/react-router'
import z from 'zod'

import { AnalyticsUsers } from '@/features/analytics'
import { getAnalyticsAccess } from '@/features/dashboard/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

const analyticsUsersSearchSchema = z.object({
  page: z.number().optional().catch(1),
  pageSize: z.number().optional().catch(undefined),
  filter: z.string().optional().catch(''),
  status: z
    .array(z.enum(['active', 'silent', 'never']))
    .optional()
    .catch([]),
  dept: z.array(z.string()).optional().catch([]),
  model: z.array(z.string()).optional().catch([]),
  range: z.number().optional().catch(undefined),
})

export const Route = createFileRoute('/_authenticated/analytics/users')({
  // 部门负责人 role < ADMIN,不能走纯角色守卫;access 探测与侧边栏共享同一
  // queryKey 缓存,fail-closed。
  beforeLoad: async ({ context }) => {
    const { auth } = useAuthStore.getState()
    if (auth.user && auth.user.role >= ROLE.ADMIN) {
      return
    }
    const res = await context.queryClient.ensureQueryData({
      queryKey: ['analytics-access'],
      queryFn: getAnalyticsAccess,
      staleTime: 300_000,
    })
    if (!res?.data?.allowed) {
      throw redirect({ to: '/403' })
    }
  },
  validateSearch: analyticsUsersSearchSchema,
  component: AnalyticsUsers,
})
