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
import type { ColumnDef } from '@tanstack/react-table'
import { AlertTriangle } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { TruncatedCell } from '@/components/data-table'
import { StatusBadge } from '@/components/status-badge'
import { TableId } from '@/components/table-id'
import { Badge } from '@/components/ui/badge'

import { formatPeriodEnd, providerLabel } from '../lib'
import type { PlanMonitorPlan } from '../types'
import { PlanMonitorRowActions } from './plan-monitor-row-actions'

export function usePlanMonitorColumns(
  onEdit: (plan: PlanMonitorPlan) => void
): ColumnDef<PlanMonitorPlan>[] {
  const { t } = useTranslation()

  return useMemo(
    (): ColumnDef<PlanMonitorPlan>[] => [
      {
        accessorKey: 'id',
        id: 'id',
        header: t('ID'),
        meta: { mobileHidden: true },
        cell: ({ row }) => <TableId value={row.original.id} />,
        size: 60,
      },
      {
        accessorKey: 'provider',
        id: 'provider',
        header: t('Provider'),
        cell: ({ row }) => (
          <span className='font-medium'>
            {providerLabel(row.original.provider)}
          </span>
        ),
        size: 120,
      },
      {
        accessorKey: 'plan_name',
        id: 'plan_name',
        header: t('Plan Name'),
        meta: { mobileTitle: true },
        cell: ({ row }) => (
          <div className='max-w-full min-w-0'>
            <div className='truncate font-medium'>
              {row.original.plan_name}
            </div>
          </div>
        ),
        size: 160,
      },
      {
        accessorKey: 'api_url',
        id: 'api_url',
        header: t('API URL'),
        meta: { mobileHidden: true },
        cell: ({ row }) => (
          <TruncatedCell>{row.original.api_url}</TruncatedCell>
        ),
        size: 200,
      },
      {
        accessorKey: 'api_key_masked',
        id: 'api_key',
        header: t('API Key'),
        meta: { mobileHidden: true },
        cell: ({ row }) => (
          <span className='text-muted-foreground font-mono text-xs'>
            {row.original.api_key_masked}
          </span>
        ),
        size: 140,
      },
      {
        accessorKey: 'refresh_interval_min',
        id: 'refresh_interval_min',
        header: t('Refresh Interval'),
        meta: { mobileHidden: true },
        cell: ({ row }) => (
          <span className='text-muted-foreground'>
            {t('{{minutes}} min', {
              minutes: row.original.refresh_interval_min,
            })}
          </span>
        ),
        size: 110,
      },
      {
        accessorKey: 'enabled',
        id: 'enabled',
        header: t('Status'),
        meta: { mobileBadge: true },
        cell: ({ row }) =>
          row.original.enabled ? (
            <StatusBadge
              label={t('Enabled')}
              variant='success'
              copyable={false}
              className='-ml-1.5'
            />
          ) : (
            <StatusBadge
              label={t('Disabled')}
              variant='neutral'
              copyable={false}
              className='-ml-1.5'
            />
          ),
        size: 80,
      },
      {
        accessorKey: 'last_fetch_time',
        id: 'last_fetch_time',
        header: t('Last Fetch'),
        meta: { mobileHidden: true },
        cell: ({ row }) => (
          <span className='text-muted-foreground'>
            {formatPeriodEnd(row.original.last_fetch_time)}
          </span>
        ),
        size: 140,
      },
      {
        accessorKey: 'last_error',
        id: 'last_error',
        header: t('Last Error'),
        meta: { mobileHidden: true },
        cell: ({ row }) => {
          const error = row.original.last_error
          if (!error) {
            return <span className='text-muted-foreground'>-</span>
          }
          return (
            <Badge variant='destructive' className='gap-1'>
              <AlertTriangle className='h-3 w-3' />
              <TruncatedCell tooltipContent={error}>
                <span className='max-w-[180px] truncate'>{error}</span>
              </TruncatedCell>
            </Badge>
          )
        },
        size: 160,
      },
      {
        id: 'actions',
        header: () => t('Actions'),
        cell: ({ row }) => (
          <PlanMonitorRowActions plan={row.original} onEdit={onEdit} />
        ),
        meta: { pinned: 'right' as const },
      },
    ],
    [t, onEdit]
  )
}
