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
import {
  Activity,
  CircleDollarSign,
  Loader2,
  ShieldAlert,
  Users,
} from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  getAnalyticsActivity,
  getAnalyticsDepartments,
  getAnalyticsHeatmap,
  getAnalyticsModels,
  getAnalyticsOverview,
  getAnalyticsTopUsers,
} from '@/features/dashboard/api'
import { formatNumber, formatPercent, formatQuota } from '@/lib/format'
import { getRollingDateRange } from '@/lib/time'

import { StatCard } from '../ui/stat-card'

import { ActivityTrend } from './activity-trend'
import { DeptComparison } from './dept-comparison'
import { ModelPreference } from './model-preference'
import { TopUsers } from './top-users'
import { UsageHeatmap } from './usage-heatmap'

const RANGE_OPTIONS = [
  { label: '7 Days', days: 7 },
  { label: '30 Days', days: 30 },
  { label: '90 Days', days: 90 },
] as const

const ANALYTICS_STALE_TIME = 60_000

export function UsageDashboard() {
  const { t } = useTranslation()
  const [rangeDays, setRangeDays] = useState(30)

  // 时间窗在 queryFn 里现算而不是冻结在 queryKey 里:React Query 的
  // staleTime/聚焦 refetch 复用旧 key,现算才能让"滚动 N 天"真正滚动。
  const rollingRange = () => {
    const { start, end } = getRollingDateRange(rangeDays)
    return {
      start_timestamp: Math.floor(start.getTime() / 1000),
      end_timestamp: Math.floor(end.getTime() / 1000),
    }
  }

  const selectData = <T,>(res: { success: boolean; data?: T }) =>
    res.success ? res.data : undefined

  const overviewQuery = useQuery({
    queryKey: ['analytics', 'overview', rangeDays],
    queryFn: () => getAnalyticsOverview(rollingRange()),
    select: selectData,
    staleTime: ANALYTICS_STALE_TIME,
  })
  const activityQuery = useQuery({
    queryKey: ['analytics', 'activity', rangeDays],
    queryFn: () => getAnalyticsActivity(rollingRange()),
    select: selectData,
    staleTime: ANALYTICS_STALE_TIME,
  })
  const topUsersQuery = useQuery({
    queryKey: ['analytics', 'top-users', rangeDays],
    queryFn: () =>
      getAnalyticsTopUsers({ ...rollingRange(), limit: 10 }),
    select: selectData,
    staleTime: ANALYTICS_STALE_TIME,
  })
  const departmentsQuery = useQuery({
    queryKey: ['analytics', 'departments', rangeDays],
    queryFn: () => getAnalyticsDepartments(rollingRange()),
    select: selectData,
    staleTime: ANALYTICS_STALE_TIME,
  })
  const modelsQuery = useQuery({
    queryKey: ['analytics', 'models', rangeDays],
    queryFn: () => getAnalyticsModels(rollingRange()),
    select: selectData,
    staleTime: ANALYTICS_STALE_TIME,
  })
  const heatmapQuery = useQuery({
    queryKey: ['analytics', 'heatmap', rangeDays],
    queryFn: () => getAnalyticsHeatmap(rollingRange()),
    select: selectData,
    staleTime: ANALYTICS_STALE_TIME,
  })

  const overview = overviewQuery.data
  const isLoading = overviewQuery.isLoading

  return (
    <div className='space-y-3'>
      <div className='flex flex-wrap items-center gap-1.5 sm:gap-2'>
        <Tabs
          value={String(rangeDays)}
          onValueChange={(value) => setRangeDays(Number(value))}
          className='shrink-0'
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
        {isLoading && (
          <Loader2 className='text-muted-foreground size-4 animate-spin' />
        )}
        <span className='text-muted-foreground ml-auto text-xs'>
          {t('Statistics use server local time')}
        </span>
      </div>

      <div className='grid grid-cols-2 gap-3 lg:grid-cols-4'>
        <StatCard
          title={t('Requests')}
          value={overview ? formatNumber(overview.request_count) : '-'}
          description={t('Total requests in range')}
          icon={Activity}
          tone='accent-1'
          loading={isLoading}
          compactMobile
        />
        <StatCard
          title={t('Active Users')}
          value={overview ? formatNumber(overview.active_users) : '-'}
          description={t('Users with at least one request')}
          icon={Users}
          tone='accent-2'
          loading={isLoading}
          compactMobile
        />
        <StatCard
          title={t('Net Consumption')}
          value={overview ? formatQuota(overview.net_quota) : '-'}
          description={t('Quota consumed minus refunds')}
          icon={CircleDollarSign}
          tone='accent-3'
          loading={isLoading}
          compactMobile
        />
        <StatCard
          title={t('Failure Rate')}
          value={overview ? formatPercent(overview.fail_rate * 100) : '-'}
          description={t('Failed requests share')}
          icon={ShieldAlert}
          tone='accent-1'
          loading={isLoading}
          compactMobile
        />
      </div>

      <ActivityTrend
        data={activityQuery.data}
        loading={activityQuery.isLoading}
      />

      <UsageHeatmap
        cells={heatmapQuery.data?.cells}
        loading={heatmapQuery.isLoading}
      />

      <div className='grid gap-3 lg:grid-cols-2'>
        <TopUsers
          users={topUsersQuery.data}
          loading={topUsersQuery.isLoading}
        />
        <ModelPreference
          models={modelsQuery.data}
          loading={modelsQuery.isLoading}
        />
      </div>

      <DeptComparison
        departments={departmentsQuery.data}
        loading={departmentsQuery.isLoading}
      />
    </div>
  )
}
