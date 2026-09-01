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
import { VChart } from '@visactor/react-vchart'
import { Users } from 'lucide-react'
import { useMemo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { IconBadge } from '@/components/ui/icon-badge'
import { Skeleton } from '@/components/ui/skeleton'
import type { AnalyticsTopUser } from '@/features/dashboard/types'
import { formatNumber, formatPercent, formatQuota } from '@/lib/format'
import { useChartTheme } from '@/lib/use-chart-theme'
import { VCHART_OPTION } from '@/lib/vchart'

interface TopUsersProps {
  users?: AnalyticsTopUser[]
  loading?: boolean
}

export function TopUsers(props: TopUsersProps) {
  const { t } = useTranslation()
  const { resolvedTheme, themeReady } = useChartTheme()

  const spec = useMemo(() => {
    // yField 用 username(唯一)而不是 display_name(可重名),否则两个同名用户
    // 会被 band 轴合并成同一行;轴标签与 tooltip 再映射回真名。
    const nameByUsername = new Map(
      (props.users ?? []).map((u) => [u.username, u.display_name || u.username])
    )
    const displayName = (key: unknown) =>
      typeof key === 'string' ? (nameByUsername.get(key) ?? key) : ''
    // 后端已按净消耗降序返回;VChart 横向条形图的左 band 轴默认 inverse,
    // domain 第一条渲染在顶部,不要 reverse(反转后第一名反而沉底)。
    const values = (props.users ?? []).map((user) => ({
      user: user.username,
      quota: user.quota,
      requests: user.request_count,
      failRate: user.fail_rate,
    }))
    return {
      type: 'bar',
      data: [{ id: 'topUsers', values }],
      direction: 'horizontal',
      xField: 'quota',
      yField: 'user',
      seriesField: 'user',
      legends: { visible: false },
      bar: { style: { cornerRadius: [0, 4, 4, 0] } },
      tooltip: {
        mark: {
          title: (datum: { user?: string } | undefined) =>
            displayName(datum?.user),
          content: [
            {
              key: t('Net Consumption'),
              value: (datum: { quota?: number } | undefined) =>
                formatQuota(datum?.quota ?? 0),
            },
            {
              key: t('Requests'),
              value: (datum: { requests?: number } | undefined) =>
                formatNumber(datum?.requests ?? 0),
            },
            {
              key: t('Failure Rate'),
              value: (datum: { failRate?: number } | undefined) =>
                formatPercent((datum?.failRate ?? 0) * 100),
            },
          ],
        },
      },
      axes: [
        {
          orient: 'bottom',
          type: 'linear',
          label: { formatMethod: (value: number) => formatQuota(value) },
        },
        {
          orient: 'left',
          type: 'band',
          label: { formatMethod: (value: string) => displayName(value) },
        },
      ],
      background: { fill: 'transparent' },
    }
  }, [props.users, t])

  let body: ReactNode
  if (props.loading) {
    body = <Skeleton className='h-full w-full' />
  } else if ((props.users?.length ?? 0) === 0) {
    body = <EmptyState icon={Users} title={t('No Data')} />
  } else {
    body = themeReady ? (
      <VChart
        key={`top-users-${resolvedTheme}`}
        spec={{
          ...spec,
          theme: resolvedTheme === 'dark' ? 'dark' : 'light',
        }}
        option={VCHART_OPTION}
      />
    ) : null
  }

  return (
    <div className='overflow-hidden rounded-lg border'>
      <div className='flex w-full items-center gap-2 border-b px-3 py-2 sm:px-5 sm:py-3'>
        <IconBadge tone='info' size='sm'>
          <Users />
        </IconBadge>
        <div className='text-sm font-semibold'>{t('Top Users')}</div>
        <span className='text-muted-foreground text-xs'>
          {t('Ranked by net consumption')}
        </span>
      </div>
      <div className='h-[320px] p-1.5 sm:h-96 sm:p-2'>{body}</div>
    </div>
  )
}
