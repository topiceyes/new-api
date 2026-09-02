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
import { useTranslation } from 'react-i18next'

import {
  DataTableColumnHeader,
  TruncatedCell,
} from '@/components/data-table'
import { LongText } from '@/components/long-text'
import { StatusBadge } from '@/components/status-badge'
import { Badge } from '@/components/ui/badge'
import { formatNumber, formatQuota } from '@/lib/format'

import {
  ANALYTICS_USER_STATUS_CONFIG,
  analyticsUserStatusRank,
} from '../constants'
import { formatDurationSeconds } from '../lib/format'
import type { AnalyticsUserTableEntry } from '../types'

function arrayFilterIncludes(
  row: { getValue: (id: string) => unknown },
  id: string,
  value: unknown
) {
  return Array.isArray(value) && value.includes(String(row.getValue(id)))
}

export function useAnalyticsUserColumns(options?: {
  showDept?: boolean
}): ColumnDef<AnalyticsUserTableEntry>[] {
  const { t } = useTranslation()
  const showDept = options?.showDept ?? true

  const columns: ColumnDef<AnalyticsUserTableEntry>[] = [
    {
      accessorKey: 'display_name',
      header: t('User'),
      cell: ({ row }) => {
        const entry = row.original
        const unbound = entry.user_id === 0
        const primary = entry.display_name || entry.username

        return (
          <div className='flex min-w-[80px] flex-col gap-1'>
            <div className='flex items-center gap-2'>
              <LongText className='max-w-[160px] font-medium'>
                {primary}
              </LongText>
              {unbound && (
                <Badge variant='outline' className='text-muted-foreground'>
                  {t('Unbound')}
                </Badge>
              )}
            </div>
            {!unbound &&
              entry.display_name &&
              entry.display_name !== entry.username && (
                <LongText className='text-muted-foreground max-w-[180px] text-xs'>
                  {entry.username}
                </LongText>
              )}
          </div>
        )
      },
      enableHiding: false,
      size: 200,
      meta: { mobileTitle: true },
    },
  ]

  if (showDept) {
    columns.push({
      accessorKey: 'dept_name',
      header: t('Department'),
      cell: ({ row }) => {
        const dept = row.getValue('dept_name') as string
        return (
          <span className='text-muted-foreground text-sm'>{dept || '-'}</span>
        )
      },
      filterFn: arrayFilterIncludes,
      size: 140,
      meta: { mobileOrder: 20 },
    })
  }

  columns.push(
    {
      accessorKey: 'status',
      header: t('Status'),
      cell: ({ row }) => {
        const config =
          ANALYTICS_USER_STATUS_CONFIG[
            row.getValue('status') as keyof typeof ANALYTICS_USER_STATUS_CONFIG
          ]
        if (!config) {
          return null
        }
        return (
          <StatusBadge
            label={t(config.labelKey)}
            variant={config.variant}
            copyable={false}
          />
        )
      },
      filterFn: arrayFilterIncludes,
      sortingFn: (a, b) =>
        analyticsUserStatusRank(a.original.status) -
        analyticsUserStatusRank(b.original.status),
      size: 110,
      meta: { mobileBadge: true },
    },
    {
      accessorKey: 'net_quota',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Net Quota')} />
      ),
      cell: ({ row }) => (
        <span className='text-sm tabular-nums'>
          {formatQuota(row.getValue('net_quota') as number)}
        </span>
      ),
      size: 130,
      meta: { mobileOrder: 30 },
    },
    {
      accessorKey: 'tokens',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Tokens')} />
      ),
      cell: ({ row }) => (
        <span className='text-sm tabular-nums'>
          {formatNumber(row.getValue('tokens') as number)}
        </span>
      ),
      size: 110,
      meta: { mobileOrder: 35 },
    },
    {
      accessorKey: 'request_count',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Requests')} />
      ),
      cell: ({ row }) => {
        const entry = row.original
        return (
          <div className='flex flex-col'>
            <span className='text-sm tabular-nums'>
              {formatNumber(entry.request_count)}
            </span>
            {entry.fail_count > 0 && (
              <span className='text-muted-foreground text-xs tabular-nums'>
                {t('Failed:')} {formatNumber(entry.fail_count)}
              </span>
            )}
          </div>
        )
      },
      size: 120,
      meta: { mobileOrder: 40 },
    },
    {
      accessorKey: 'active_days',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Active Days')} />
      ),
      cell: ({ row }) => (
        <span className='text-sm tabular-nums'>
          {row.getValue('active_days') as number}
        </span>
      ),
      size: 100,
      meta: { mobileOrder: 50 },
    },
    {
      accessorKey: 'last_active_date',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Last Active')} />
      ),
      cell: ({ row }) => {
        const date = row.getValue('last_active_date') as string
        return (
          <span className='text-muted-foreground text-sm tabular-nums'>
            {date || '-'}
          </span>
        )
      },
      size: 130,
      meta: { mobileOrder: 60 },
    },
    {
      accessorKey: 'top_model',
      header: t('Top Model'),
      cell: ({ row }) => {
        const model = row.getValue('top_model') as string
        if (!model) {
          return <span className='text-muted-foreground text-sm'>-</span>
        }
        return (
          <TruncatedCell className='max-w-[180px] font-mono text-xs'>
            {model}
          </TruncatedCell>
        )
      },
      filterFn: arrayFilterIncludes,
      size: 180,
      meta: { mobileHidden: true },
    },
    {
      accessorKey: 'avg_use_time',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Avg Duration')} />
      ),
      cell: ({ row }) => {
        const entry = row.original
        return (
          <span className='text-muted-foreground text-sm tabular-nums'>
            {entry.request_count > 0
              ? formatDurationSeconds(entry.avg_use_time)
              : '-'}
          </span>
        )
      },
      size: 110,
      meta: { mobileHidden: true },
    }
  )

  return columns
}
