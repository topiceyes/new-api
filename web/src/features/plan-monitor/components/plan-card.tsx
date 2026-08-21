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
import { AlertTriangle, LineChart } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Progress,
  ProgressIndicator,
  ProgressTrack,
} from '@/components/ui/progress'
import { Separator } from '@/components/ui/separator'

import {
  formatPeriodEnd,
  periodLabelKey,
  providerLabel,
  usageLevel,
  type UsageLevel,
} from '../lib'
import type { PlanMonitorUsageView } from '../types'
import { UsageTrendDialog, type PlanTrendScope } from './usage-trend-dialog'

// 共享卡片三件套:UsageRow / PlanCard / PlanGroupGrid。
// 管理员概览页与用户端「套餐余量」页共用。
// lastError / enabled 是显式可选 prop:公开页不传(裁剪 DTO 里没有这些字段),
// 不能从 plan 上隐式推断(undefined !== '' 为 true 会误挂错误徽标)。

const LEVEL_BAR_CLASS: Record<UsageLevel, string> = {
  green: 'bg-green-500',
  yellow: 'bg-yellow-500',
  red: 'bg-red-500',
}

// PlanCard 只依赖这三个字段;管理员的完整 DTO 与用户端裁剪 DTO 都满足。
export interface PlanCardPlan {
  id: number
  plan_name: string
  usages: PlanMonitorUsageView[]
}

function UsageRow({
  usage,
  onShowTrend,
}: {
  usage: PlanMonitorUsageView
  onShowTrend: () => void
}) {
  const { t } = useTranslation()
  const level = usageLevel(usage.used_percent)
  return (
    <div className='space-y-0.5'>
      <div className='flex items-center justify-between text-xs'>
        <span className='text-muted-foreground flex items-center gap-1'>
          {t(periodLabelKey(usage.period))}
          <Button
            variant='ghost'
            size='icon'
            className='h-5 w-5'
            title={t('Usage Trend')}
            onClick={onShowTrend}
          >
            <LineChart className='h-3.5 w-3.5' />
          </Button>
        </span>
        <span className='font-medium tabular-nums'>
          {usage.used_percent.toFixed(1)}%
        </span>
      </div>
      <Progress value={usage.used_percent} className='block'>
        <ProgressTrack>
          <ProgressIndicator className={LEVEL_BAR_CLASS[level]} />
        </ProgressTrack>
      </Progress>
      <div className='text-muted-foreground text-xs'>
        {t('Resets at')} {formatPeriodEnd(usage.period_end_time)}
      </div>
    </div>
  )
}

function PlanStatusBadge({
  lastError,
  enabled,
}: {
  lastError: string
  enabled: boolean
}) {
  const { t } = useTranslation()
  if (lastError !== '') {
    return (
      <Badge variant='destructive' className='gap-1'>
        <AlertTriangle className='h-3 w-3' />
        {t('Fetch failed')}
      </Badge>
    )
  }
  if (!enabled) {
    return <Badge variant='outline'>{t('Disabled')}</Badge>
  }
  return null
}

export function PlanCard({
  plan,
  lastError,
  enabled,
  trendScope,
}: {
  plan: PlanCardPlan
  // 管理员页传入:错误信息与启停状态用于徽标/正文展示;公开页不传。
  lastError?: string
  enabled?: boolean
  trendScope: PlanTrendScope
}) {
  const { t } = useTranslation()
  const errorText = lastError ?? ''
  const hasError = errorText !== ''
  const showStatusBadge = lastError !== undefined || enabled !== undefined
  const [trendPeriod, setTrendPeriod] = useState<string | null>(null)
  return (
    <Card className='gap-3 py-3'>
      <CardHeader className='flex flex-row items-center justify-between space-y-0 px-4'>
        <CardTitle className='text-sm'>{plan.plan_name}</CardTitle>
        {showStatusBadge && (
          <PlanStatusBadge lastError={errorText} enabled={enabled ?? true} />
        )}
      </CardHeader>
      <CardContent className='space-y-2.5 px-4'>
        {plan.usages.length === 0 ? (
          <div className='text-muted-foreground text-sm'>
            {hasError
              ? errorText
              : t('No usage data yet. It will appear after the first fetch.')}
          </div>
        ) : (
          plan.usages.map((u) => (
            <UsageRow
              key={u.period}
              usage={u}
              onShowTrend={() => setTrendPeriod(u.period)}
            />
          ))
        )}
        {hasError && plan.usages.length > 0 ? (
          <div className='text-destructive text-xs'>{errorText}</div>
        ) : null}
      </CardContent>
      <UsageTrendDialog
        planId={plan.id}
        planName={plan.plan_name}
        period={trendPeriod ?? ''}
        scope={trendScope}
        open={trendPeriod !== null}
        onOpenChange={(open) => {
          if (!open) setTrendPeriod(null)
        }}
      />
    </Card>
  )
}

export function PlanGroupGrid<P extends PlanCardPlan>({
  group,
  trendScope,
  lastErrorOf,
  enabledOf,
}: {
  group: { provider: string; plans: P[] }
  trendScope: PlanTrendScope
  // 管理员页传索引函数取管理字段;公开页不传。
  lastErrorOf?: (plan: P) => string | undefined
  enabledOf?: (plan: P) => boolean | undefined
}) {
  return (
    <section>
      <h2 className='text-base font-semibold'>
        {providerLabel(group.provider)}
      </h2>
      <Separator className='my-2' />
      <div className='grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3'>
        {group.plans.map((plan) => (
          <PlanCard
            key={plan.id}
            plan={plan}
            lastError={lastErrorOf?.(plan)}
            enabled={enabledOf?.(plan)}
            trendScope={trendScope}
          />
        ))}
      </div>
    </section>
  )
}
