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
import { RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'

import { getPublicOverview } from './api'
import { PlanGroupGrid } from './components/plan-card'
import type { PlanBalanceGroup } from './types'

// 用户端「套餐余量」:展示管理员标记为公开且启用的上游套餐用量,
// 与管理员概览共用卡片组件,但无任何管理入口/字段。
export function PlanMonitorBalance() {
  const { t } = useTranslation()
  const { data, isLoading, refetch, isFetching } = useQuery({
    queryKey: ['plan-balance-overview'],
    queryFn: async () => {
      const res = await getPublicOverview()
      if (!res.success) throw new Error(res.message)
      return res.data.groups
    },
    refetchInterval: 60_000,
  })

  const groups: PlanBalanceGroup[] = data ?? []

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Plan Balance')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
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
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        {isLoading ? (
          <div className='text-muted-foreground py-12 text-center'>
            {t('Loading...')}
          </div>
        ) : groups.length === 0 ? (
          <div className='text-muted-foreground py-12 text-center'>
            {t('No public plans available yet.')}
          </div>
        ) : (
          <div className='space-y-6'>
            {groups.map((group) => (
              <PlanGroupGrid
                key={group.provider}
                group={group}
                trendScope='public'
              />
            ))}
          </div>
        )}
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
