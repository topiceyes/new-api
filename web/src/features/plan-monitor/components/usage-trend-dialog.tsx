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
import { VChart } from '@visactor/react-vchart'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import dayjs from '@/lib/dayjs'
import { useChartTheme } from '@/lib/use-chart-theme'
import { VCHART_OPTION } from '@/lib/vchart'

import { getPlanHistory, getPublicPlanHistory } from '../api'
import { periodLabelKey } from '../lib'
import type { PlanHistoryRange } from '../types'

const RANGE_LABEL_KEYS: Record<PlanHistoryRange, string> = {
  '24h': 'Last 24 hours',
  '7d': 'Last 7 days',
  '30d': 'Last 30 days',
}

// admin 走 /admin/plans/:id/history(管理员概览页);
// public 走 /plans/:id/history(用户端「套餐余量」页,服务端校验 is_public)。
export type PlanTrendScope = 'admin' | 'public'

function formatPointTime(ts: number, range: PlanHistoryRange): string {
  const d = dayjs(ts * 1000)
  return range === '24h' ? d.format('MM-DD HH:mm') : d.format('MM-DD HH:00')
}

interface UsageTrendDialogProps {
  planId: number
  planName: string
  period: string
  scope: PlanTrendScope
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function UsageTrendDialog({
  planId,
  planName,
  period,
  scope,
  open,
  onOpenChange,
}: UsageTrendDialogProps) {
  const { t } = useTranslation()
  const { resolvedTheme, themeReady } = useChartTheme()
  const [range, setRange] = useState<PlanHistoryRange>('24h')

  const { data, isLoading } = useQuery({
    queryKey: ['plan-monitor-history', scope, planId, period, range],
    queryFn: async () => {
      const res =
        scope === 'public'
          ? await getPublicPlanHistory(planId, period, range)
          : await getPlanHistory(planId, period, range)
      if (!res.success) throw new Error(res.message)
      return res.data.points
    },
    enabled: open && planId > 0 && period !== '',
  })

  const textColor =
    resolvedTheme === 'dark'
      ? 'rgba(255, 255, 255, 0.68)'
      : 'rgba(15, 23, 42, 0.58)'
  const gridColor =
    resolvedTheme === 'dark'
      ? 'rgba(255, 255, 255, 0.12)'
      : 'rgba(15, 23, 42, 0.12)'

  const points = data ?? []

  const spec = useMemo(() => {
    if (points.length === 0) return null
    const values = points.map((p) => ({
      time: formatPointTime(p.ts, range),
      used: Math.min(100, Math.max(0, p.used_percent)),
    }))
    return {
      type: 'area' as const,
      data: [{ id: 'usage', values }],
      xField: 'time',
      yField: 'used',
      smooth: true,
      line: { style: { stroke: '#3b82f6', lineWidth: 2 } },
      area: { style: { fill: '#3b82f6', fillOpacity: 0.15 } },
      point: { visible: false },
      legends: { visible: false },
      tooltip: {
        mark: {
          title: { value: (d: { time: string }) => d.time },
          content: [
            {
              key: t('Used'),
              value: (d: { used: number }) => `${d.used.toFixed(1)}%`,
            },
          ],
        },
      },
      axes: [
        {
          orient: 'bottom',
          label: { style: { fill: textColor, fontSize: 10 } },
          tick: { visible: false },
        },
        {
          orient: 'left',
          min: 0,
          max: 100,
          label: {
            formatMethod: (val: number | string) => `${val}%`,
            style: { fill: textColor, fontSize: 10 },
          },
          grid: {
            visible: true,
            style: { lineDash: [3, 3], stroke: gridColor },
          },
        },
      ],
    }
  }, [points, range, textColor, gridColor, t])

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={`${planName} · ${t(periodLabelKey(period))}`}
      description={t('Usage Trend')}
      contentClassName='sm:max-w-2xl'
      contentHeight='auto'
    >
      <div className='space-y-3 py-2'>
        <Tabs
          value={range}
          onValueChange={(v) => setRange(v as PlanHistoryRange)}
        >
          <TabsList>
            {(Object.keys(RANGE_LABEL_KEYS) as PlanHistoryRange[]).map((r) => (
              <TabsTrigger key={r} value={r}>
                {t(RANGE_LABEL_KEYS[r])}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
        {isLoading ? (
          <div className='text-muted-foreground flex h-64 items-center justify-center text-sm'>
            {t('Loading...')}
          </div>
        ) : points.length === 0 ? (
          <div className='text-muted-foreground flex h-64 items-center justify-center rounded-lg border text-sm'>
            {t('No history data yet. It will appear after a few refreshes.')}
          </div>
        ) : (
          <div className='h-64'>
            {themeReady && spec && (
              <VChart
                key={`usage-trend-${resolvedTheme}`}
                spec={{
                  ...spec,
                  theme: resolvedTheme === 'dark' ? 'dark' : 'light',
                  background: 'transparent',
                }}
                option={VCHART_OPTION}
              />
            )}
          </div>
        )}
      </div>
    </Dialog>
  )
}
