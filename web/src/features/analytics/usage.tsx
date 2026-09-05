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
import { Suspense, lazy } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'

// 使用分析看板从「数据看板」迁入「统计分析」菜单。组件保留在 dashboard 特性
// 下(图表与 stat-card 共址),这里只做页面壳。惰性加载与控制台首屏解耦。
const UsageDashboard = lazy(() =>
  import('@/features/dashboard/components/usage/usage-dashboard').then((m) => ({
    default: m.UsageDashboard,
  }))
)

export function AnalyticsUsage() {
  const { t } = useTranslation()

  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>{t('Usage Analytics')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <Suspense fallback={null}>
          <UsageDashboard />
        </Suspense>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
