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
import { Link } from '@tanstack/react-router'
import { AlertTriangle, RefreshCw, Settings } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Progress,
  ProgressIndicator,
  ProgressTrack,
} from '@/components/ui/progress'
import { Separator } from '@/components/ui/separator'

import { getOverview } from './api'
import {
  formatPeriodEnd,
  periodLabelKey,
  providerLabel,
  usageLevel,
  type UsageLevel,
} from './lib'
import type { PlanMonitorOverviewItem, PlanMonitorUsageView } from './types'

const LEVEL_BAR_CLASS: Record<UsageLevel, string> = {
  green: 'bg-green-500',
  yellow: 'bg-yellow-500',
  red: 'bg-red-500',
}

function UsageRow({ usage }: { usage: PlanMonitorUsageView }) {
  const { t } = useTranslation()
  const level = usageLevel(usage.used_percent)
  return (
    <div className='space-y-0.5'>
      <div className='flex items-center justify-between text-xs'>
        <span className='text-muted-foreground'>
          {t(periodLabelKey(usage.period))}
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

function PlanStatusBadge({ plan }: { plan: PlanMonitorOverviewItem }) {
  const { t } = useTranslation()
  if (plan.last_error !== '') {
    return (
      <Badge variant='destructive' className='gap-1'>
        <AlertTriangle className='h-3 w-3' />
        {t('Fetch failed')}
      </Badge>
    )
  }
  if (!plan.enabled) {
    return <Badge variant='outline'>{t('Disabled')}</Badge>
  }
  return null
}

function PlanCard({ plan }: { plan: PlanMonitorOverviewItem }) {
  const { t } = useTranslation()
  const hasError = plan.last_error !== ''
  return (
    <Card className='gap-3 py-3'>
      <CardHeader className='flex flex-row items-center justify-between space-y-0 px-4'>
        <CardTitle className='text-sm'>{plan.plan_name}</CardTitle>
        <PlanStatusBadge plan={plan} />
      </CardHeader>
      <CardContent className='space-y-2.5 px-4'>
        {plan.usages.length === 0 ? (
          <div className='text-muted-foreground text-sm'>
            {hasError
              ? plan.last_error
              : t('No usage data yet. It will appear after the first fetch.')}
          </div>
        ) : (
          plan.usages.map((u) => <UsageRow key={u.period} usage={u} />)
        )}
        {hasError && plan.usages.length > 0 ? (
          <div className='text-destructive text-xs'>{plan.last_error}</div>
        ) : null}
      </CardContent>
    </Card>
  )
}

export function PlanMonitorOverview() {
  const { t } = useTranslation()
  const { data, isLoading, refetch, isFetching } = useQuery({
    queryKey: ['plan-monitor-overview'],
    queryFn: async () => {
      const res = await getOverview()
      if (!res.success) throw new Error(res.message)
      return res.data.groups
    },
    refetchInterval: 60_000,
  })

  const groups = data ?? []

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Plan Usage Monitor')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <div className='flex items-center gap-2'>
          <Link to='/plan-monitor'>
            <Button variant='outline' size='sm'>
              <Settings className='mr-2 h-4 w-4' />
              {t('Plan Monitor Settings')}
            </Button>
          </Link>
          <Button
            variant='outline'
            size='sm'
            onClick={() => refetch()}
            disabled={isFetching}
          >
            <RefreshCw
              className={`mr-2 h-4 w-4 ${isFetching ? 'animate-spin' : ''}`}
            />
            {t('Refresh')}
          </Button>
        </div>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <PlanOverviewBody
          isLoading={isLoading}
          groups={groups}
        />
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

function PlanOverviewBody({
  isLoading,
  groups,
}: {
  isLoading: boolean
  groups: { provider: string; plans: PlanMonitorOverviewItem[] }[]
}) {
  const { t } = useTranslation()
  if (isLoading) {
    return (
      <div className='text-muted-foreground py-12 text-center'>
        {t('Loading...')}
      </div>
    )
  }
  if (groups.length === 0) {
    return (
      <div className='text-muted-foreground py-12 text-center'>
        {t('No plans configured. Add one in Plan Monitor settings.')}
      </div>
    )
  }
  return (
    <div className='space-y-6'>
      {groups.map((group) => (
        <section key={group.provider}>
          <h2 className='text-base font-semibold'>
            {providerLabel(group.provider)}
          </h2>
          <Separator className='my-2' />
          <div className='grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3'>
            {group.plans.map((plan) => (
              <PlanCard key={plan.id} plan={plan} />
            ))}
          </div>
        </section>
      ))}
    </div>
  )
}
