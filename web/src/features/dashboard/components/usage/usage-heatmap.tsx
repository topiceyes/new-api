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
import { CalendarClock } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { IconBadge } from '@/components/ui/icon-badge'
import { Skeleton } from '@/components/ui/skeleton'
import type { AnalyticsHeatmapCell } from '@/features/dashboard/types'
import { formatNumber } from '@/lib/format'
import { useChartTheme } from '@/lib/use-chart-theme'
import { VCHART_OPTION } from '@/lib/vchart'

// Backend day_of_week follows Go's time.Weekday: 0 = Sunday.
const DAY_LABEL_KEYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']

interface UsageHeatmapProps {
  cells?: AnalyticsHeatmapCell[]
  loading?: boolean
}

export function UsageHeatmap(props: UsageHeatmapProps) {
  const { t } = useTranslation()
  const { resolvedTheme, themeReady } = useChartTheme()

  const spec = useMemo(() => {
    const dayLabels = DAY_LABEL_KEYS.map((key) => t(key))
    const values = (props.cells ?? []).map((cell) => ({
      hour: String(cell.hour),
      day: dayLabels[cell.day_of_week] ?? String(cell.day_of_week),
      value: cell.request_count,
      quota: cell.quota,
    }))
    const maxValue = values.reduce((max, v) => Math.max(max, v.value), 0)
    return {
      type: 'heatmap',
      data: [{ id: 'usageHeatmap', values }],
      xField: 'hour',
      yField: 'day',
      valueField: 'value',
      cell: { style: { stroke: 'transparent' } },
      color: {
        type: 'linear',
        domain: [0, Math.max(maxValue, 1)],
        range:
          resolvedTheme === 'dark'
            ? ['#1e293b', '#0ea5e9']
            : ['#f1f5f9', '#0284c7'],
      },
      axes: [
        {
          orient: 'bottom',
          type: 'band',
          domainLine: { visible: false },
          tick: { visible: false },
          grid: { visible: false },
        },
        {
          orient: 'left',
          type: 'band',
          // 垂直图左 band 轴默认把 domain[0](周日)画在底部,与日历习惯相反;
          // inverse 后周日在上、周六在下(GitHub 风格)。
          inverse: true,
          domainLine: { visible: false },
          tick: { visible: false },
          grid: { visible: false },
        },
      ],
      legends: {
        visible: true,
        type: 'color',
        orient: 'bottom',
        position: 'end',
        field: 'value',
      },
      tooltip: {
        mark: {
          content: [
            {
              key: t('Requests'),
              value: (datum: { value?: number } | undefined) =>
                formatNumber(datum?.value ?? 0),
            },
          ],
        },
      },
      background: { fill: 'transparent' },
    }
  }, [props.cells, resolvedTheme, t])

  return (
    <div className='overflow-hidden rounded-lg border'>
      <div className='flex w-full items-center gap-2 border-b px-3 py-2 sm:px-5 sm:py-3'>
        <IconBadge tone='info' size='sm'>
          <CalendarClock />
        </IconBadge>
        <div className='text-sm font-semibold'>{t('Usage Heatmap')}</div>
        <span className='text-muted-foreground text-xs'>
          {t('Requests by day of week and hour')}
        </span>
      </div>
      <div className='h-[260px] p-1.5 sm:h-72 sm:p-2'>
        {props.loading ? (
          <Skeleton className='h-full w-full' />
        ) : (
          themeReady && (
            <VChart
              key={`usage-heatmap-${resolvedTheme}`}
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
