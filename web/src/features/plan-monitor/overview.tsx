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
import { RefreshCw, Settings } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'

import { getOverview } from './api'
import { PlanGroupGrid } from './components/plan-card'
import type { PlanMonitorOverviewItem } from './types'

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
        <PlanGroupGrid
          key={group.provider}
          group={group}
          trendScope='admin'
          lastErrorOf={(p) => p.last_error}
          enabledOf={(p) => p.enabled}
        />
      ))}
    </div>
  )
}
