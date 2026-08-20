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
import { Link } from '@tanstack/react-router'
import { BarChart3, Plus } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'

import { PlanMonitorMutateDrawer } from './components/plan-monitor-mutate-drawer'
import { PlanMonitorTable } from './components/plan-monitor-table'
import type { PlanMonitorPlan } from './types'

export function PlanMonitorSettings() {
  const { t } = useTranslation()
  const [drawerOpen, setDrawerOpen] = useState(false)
  const [currentRow, setCurrentRow] = useState<PlanMonitorPlan | undefined>(
    undefined
  )

  const handleCreate = () => {
    setCurrentRow(undefined)
    setDrawerOpen(true)
  }

  const handleEdit = (plan: PlanMonitorPlan) => {
    setCurrentRow(plan)
    setDrawerOpen(true)
  }

  return (
    <>
      <SectionPageLayout fixedContent>
        <SectionPageLayout.Title>{t('Plan Monitor')}</SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <div className='flex items-center gap-2'>
            <Link to='/plan-monitor/overview'>
              <Button variant='outline'>
                <BarChart3 className='mr-2 h-4 w-4' />
                {t('View Usage')}
              </Button>
            </Link>
            <Button onClick={handleCreate}>
              <Plus className='mr-2 h-4 w-4' />
              {t('Add Plan')}
            </Button>
          </div>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <PlanMonitorTable onEdit={handleEdit} />
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <PlanMonitorMutateDrawer
        open={drawerOpen}
        onOpenChange={setDrawerOpen}
        currentRow={currentRow}
      />
    </>
  )
}
