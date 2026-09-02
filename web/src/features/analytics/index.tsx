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
import { useQuery } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { getRollingDateRange } from '@/lib/time'

import { getAnalyticsUserTable } from './api'
import { AnalyticsUserTable } from './components/user-table'

const route = getRouteApi('/_authenticated/analytics/users')

const RANGE_OPTIONS = [
  { label: '7 Days', days: 7 },
  { label: '30 Days', days: 30 },
  { label: '90 Days', days: 90 },
] as const

const DEFAULT_RANGE_DAYS = 30
const ANALYTICS_STALE_TIME = 60_000

export function AnalyticsUsers() {
  const { t } = useTranslation()
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const rangeDays = search.range ?? DEFAULT_RANGE_DAYS

  const setRangeDays = (days: number) => {
    navigate({
      search: (prev) => ({
        ...(prev as Record<string, unknown>),
        range: days === DEFAULT_RANGE_DAYS ? undefined : days,
      }),
    })
  }

  // 时间窗在 queryFn 里现算而不是冻结在 queryKey 里(同 usage-dashboard 先例)。
  const tableQuery = useQuery({
    queryKey: ['analytics', 'user-table', rangeDays],
    queryFn: () => {
      const { start, end } = getRollingDateRange(rangeDays)
      return getAnalyticsUserTable({
        start_timestamp: Math.floor(start.getTime() / 1000),
        end_timestamp: Math.floor(end.getTime() / 1000),
      })
    },
    select: (res) => (res.success ? (res.data ?? []) : []),
    staleTime: ANALYTICS_STALE_TIME,
  })

  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>{t('User Analytics')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Tabs
          value={String(rangeDays)}
          onValueChange={(value) => setRangeDays(Number(value))}
        >
          <TabsList>
            {RANGE_OPTIONS.map((opt) => (
              <TabsTrigger
                key={opt.days}
                value={String(opt.days)}
                className='px-2.5 text-xs'
              >
                {t(opt.label)}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <AnalyticsUserTable
          entries={tableQuery.data ?? []}
          isLoading={tableQuery.isLoading}
          isFetching={tableQuery.isFetching}
        />
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
