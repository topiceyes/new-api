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

import { PlanMonitorBalance } from '@/features/plan-monitor/balance'
import { isSidebarModuleEnabled } from '@/lib/nav-modules'

export const Route = createFileRoute('/_authenticated/plan-monitor/balance')({
  beforeLoad: () => {
    // 模块开关关闭时挡直接 URL 访问(侧边栏入口也会被同一配置隐藏)。
    if (!isSidebarModuleEnabled('console', 'plan_balance')) {
      throw redirect({ to: '/403' })
    }
  },
  component: PlanMonitorBalance,
})
