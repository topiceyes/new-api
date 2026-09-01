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
import { Boxes } from 'lucide-react'
import { useMemo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { IconBadge } from '@/components/ui/icon-badge'
import { Skeleton } from '@/components/ui/skeleton'
import type { AnalyticsModel } from '@/features/dashboard/types'
import { formatNumber, formatQuota } from '@/lib/format'
import { useChartTheme } from '@/lib/use-chart-theme'
import { VCHART_OPTION } from '@/lib/vchart'

interface ModelPreferenceProps {
  models?: AnalyticsModel[]
  loading?: boolean
}

export function ModelPreference(props: ModelPreferenceProps) {
  const { t } = useTranslation()
  const { resolvedTheme, themeReady } = useChartTheme()

  const spec = useMemo(() => {
    const values = (props.models ?? []).map((model) => ({
      model: model.model_name === 'other' ? t('Other') : model.model_name,
      quota: model.quota,
      requests: model.request_count,
    }))
    return {
      type: 'pie',
      data: [{ id: 'modelPreference', values }],
      outerRadius: 0.8,
      innerRadius: 0.5,
      padAngle: 0.6,
      valueField: 'quota',
      categoryField: 'model',
      legends: { visible: true, orient: 'bottom' },
      label: { visible: false },
      tooltip: {
        mark: {
          content: [
            {
              key: (datum: { model?: string } | undefined) =>
                datum?.model ?? '',
              value: (datum: { quota?: number } | undefined) =>
                formatQuota(datum?.quota ?? 0),
            },
            {
              key: t('Requests'),
              value: (datum: { requests?: number } | undefined) =>
                formatNumber(datum?.requests ?? 0),
            },
          ],
        },
      },
      background: { fill: 'transparent' },
    }
  }, [props.models, t])

  // 饼图以 quota 为扇区值;全零(纯免费/缓存流量)时一个扇区都画不出来,
  // 只剩图例,用户会以为图坏了 —— 显式空态。
  const totalQuota = (props.models ?? []).reduce((sum, m) => sum + m.quota, 0)
  const isEmpty = !props.loading && totalQuota <= 0

  let body: ReactNode
  if (props.loading) {
    body = <Skeleton className='h-full w-full' />
  } else if (isEmpty) {
    body = <EmptyState icon={Boxes} title={t('No Data')} />
  } else {
    body = themeReady ? (
      <VChart
        key={`model-preference-${resolvedTheme}`}
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
          <Boxes />
        </IconBadge>
        <div className='text-sm font-semibold'>{t('Model Preference')}</div>
        <span className='text-muted-foreground text-xs'>
          {t('Consumption share by model')}
        </span>
      </div>
      <div className='h-[320px] p-1.5 sm:h-96 sm:p-2'>{body}</div>
    </div>
  )
}
