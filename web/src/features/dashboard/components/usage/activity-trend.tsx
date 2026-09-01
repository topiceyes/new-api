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
import { TrendingUp } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { IconBadge } from '@/components/ui/icon-badge'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { AnalyticsActivity } from '@/features/dashboard/types'
import { formatNumber, formatQuota } from '@/lib/format'
import { useChartTheme } from '@/lib/use-chart-theme'
import { VCHART_OPTION } from '@/lib/vchart'

type ActivityMetric = 'active_users' | 'request_count' | 'quota'

const METRIC_OPTIONS: { value: ActivityMetric; labelKey: string }[] = [
  { value: 'active_users', labelKey: 'Active Users' },
  { value: 'request_count', labelKey: 'Requests' },
  { value: 'quota', labelKey: 'Net Consumption' },
]

const METRIC_LABEL_KEYS: Record<ActivityMetric, string> = {
  active_users: 'Active Users',
  request_count: 'Requests',
  quota: 'Net Consumption',
}

interface ActivityTrendProps {
  data?: AnalyticsActivity
  loading?: boolean
}

export function ActivityTrend(props: ActivityTrendProps) {
  const { t } = useTranslation()
  const { resolvedTheme, themeReady } = useChartTheme()
  const [metric, setMetric] = useState<ActivityMetric>('active_users')

  const spec = useMemo(() => {
    const days = props.data?.days ?? []
    // xField 用完整 'YYYY-MM-DD'(字典序=时间序,跨年也不错乱),
    // 轴标签与 tooltip 再切成 'MM-DD' 展示。
    const values = days.map((d) => ({
      date: d.date,
      value: metric === 'quota' ? d.quota : d[metric],
    }))
    return {
      type: 'area',
      data: [{ id: 'activityTrend', values }],
      xField: 'date',
      yField: 'value',
      point: { visible: false },
      line: { style: { curveType: 'monotone' } },
      area: { style: { fillOpacity: 0.15 } },
      tooltip: {
        mark: {
          title: (datum: { date?: string } | undefined) => datum?.date ?? '',
          content: [
            {
              key: t(METRIC_LABEL_KEYS[metric]),
              value: (datum: { value?: number } | undefined) =>
                metric === 'quota'
                  ? formatQuota(datum?.value ?? 0)
                  : formatNumber(datum?.value ?? 0),
            },
          ],
        },
      },
      axes: [
        {
          orient: 'bottom',
          type: 'band',
          label: {
            formatMethod: (value: string) =>
              typeof value === 'string' ? value.slice(5) : value,
          },
        },
        {
          orient: 'left',
          type: 'linear',
          label: {
            formatMethod: (value: number) =>
              metric === 'quota' ? formatQuota(value) : formatNumber(value),
          },
        },
      ],
      background: { fill: 'transparent' },
    }
  }, [props.data, metric, t])

  return (
    <div className='overflow-hidden rounded-lg border'>
      <div className='flex w-full flex-wrap items-center gap-2 border-b px-3 py-2 sm:px-5 sm:py-3'>
        <IconBadge tone='info' size='sm'>
          <TrendingUp />
        </IconBadge>
        <div className='text-sm font-semibold'>{t('Activity Trend')}</div>
        {props.data && (
          <span className='text-muted-foreground text-xs'>
            {t('WAU')}: {formatNumber(props.data.wau)} · {t('MAU')}:{' '}
            {formatNumber(props.data.mau)}
          </span>
        )}
        <Tabs
          value={metric}
          onValueChange={(value) => setMetric(value as ActivityMetric)}
          className='ml-auto shrink-0'
        >
          <TabsList>
            {METRIC_OPTIONS.map((opt) => (
              <TabsTrigger
                key={opt.value}
                value={opt.value}
                className='px-2.5 text-xs'
              >
                {t(opt.labelKey)}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
      </div>
      <div className='h-[280px] p-1.5 sm:h-80 sm:p-2'>
        {props.loading ? (
          <Skeleton className='h-full w-full' />
        ) : (
          themeReady && (
            <VChart
              key={`activity-${metric}-${resolvedTheme}`}
              spec={{
                ...spec,
                theme: resolvedTheme === 'dark' ? 'dark' : 'light',
              }}
              option={VCHART_OPTION}
            />
          )
        )}
      </div>
    </div>
  )
}
