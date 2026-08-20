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
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { DataTablePage, useDataTable } from '@/components/data-table'

import { listPlans } from '../api'
import { usePlanMonitorColumns } from './plan-monitor-columns'
import type { PlanMonitorPlan } from '../types'

interface Props {
  onEdit: (plan: PlanMonitorPlan) => void
}

export function PlanMonitorTable({ onEdit }: Props) {
  const { t } = useTranslation()
  const columns = usePlanMonitorColumns(onEdit)

  const { data, isLoading } = useQuery({
    queryKey: ['plan-monitor-plans'],
    queryFn: async () => {
      const result = await listPlans()
      if (!result.success) throw new Error(result.message)
      return result.data
    },
    placeholderData: (prev) => prev,
  })

  const plans = useMemo(() => data?.plans ?? [], [data])

  const { table } = useDataTable({
    data: plans,
    columns,
    withFilteredRowModel: false,
    withFacetedRowModel: false,
  })

  return (
    <DataTablePage
      table={table}
      columns={columns}
      isLoading={isLoading}
      emptyTitle={t('No monitored plans yet')}
      emptyDescription={t(
        'Click "Add Plan" to create your first plan monitor entry'
      )}
      skeletonKeyPrefix='plan-monitor-skeleton'
      applyHeaderSize
    />
  )
}
