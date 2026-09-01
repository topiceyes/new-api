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
*/
import type { ColumnDef } from '@tanstack/react-table'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { TruncatedCell } from '@/components/data-table'
import { StatusBadge } from '@/components/status-badge'
import { TableId } from '@/components/table-id'
import { Button } from '@/components/ui/button'

import type { AuditEvent } from '../types'

const severityVariant: Record<
  string,
  'info' | 'warning' | 'danger' | 'neutral'
> = {
  info: 'info',
  warning: 'warning',
  critical: 'danger',
}

function formatTime(ts: number): string {
  if (!ts) return '-'
  return new Date(ts * 1000).toLocaleString()
}

export function useAuditEventColumns(
  onView: (event: AuditEvent) => void
): ColumnDef<AuditEvent>[] {
  const { t } = useTranslation()

  return useMemo(
    (): ColumnDef<AuditEvent>[] => [
      {
        accessorKey: 'id',
        id: 'id',
        header: t('ID'),
        cell: ({ row }) => <TableId value={row.original.id} />,
        size: 60,
      },
      {
        accessorKey: 'created_at',
        id: 'created_at',
        header: t('Time'),
        cell: ({ row }) => (
          <span className='text-muted-foreground'>
            {formatTime(row.original.created_at)}
          </span>
        ),
        size: 160,
      },
      {
        accessorKey: 'event_type',
        id: 'event_type',
        header: t('Event Type'),
        cell: ({ row }) => (
          <span className='font-mono text-xs'>{row.original.event_type}</span>
        ),
        size: 150,
      },
      {
        accessorKey: 'severity',
        id: 'severity',
        header: t('Severity'),
        cell: ({ row }) => (
          <StatusBadge
            label={t(row.original.severity)}
            variant={severityVariant[row.original.severity] ?? 'neutral'}
            copyable={false}
            className='-ml-1.5'
          />
        ),
        size: 90,
      },
      {
        accessorKey: 'username',
        id: 'username',
        header: t('User'),
        cell: ({ row }) => {
          // 真名为主:显示真实姓名,username 以小字辅助
          const { username, display_name: displayName } = row.original
          const primary = displayName || username || '-'
          return (
            <span className='font-medium' title={displayName ? username : undefined}>
              {primary}
              {displayName && displayName !== username && (
                <span className='text-muted-foreground ml-1 text-xs'>
                  ({username})
                </span>
              )}
            </span>
          )
        },
        size: 120,
      },
      {
        accessorKey: 'token_name',
        id: 'token_name',
        header: t('Token'),
        cell: ({ row }) => (
          <span className='text-muted-foreground'>
            {row.original.token_name || `#${row.original.token_id}`}
          </span>
        ),
        size: 110,
      },
      {
        accessorKey: 'model_name',
        id: 'model_name',
        header: t('Model'),
        cell: ({ row }) => (
          <TruncatedCell>{row.original.model_name || '-'}</TruncatedCell>
        ),
        size: 140,
      },
      {
        accessorKey: 'rule_name',
        id: 'rule_name',
        header: t('Rule'),
        cell: ({ row }) => (
          <span>{row.original.rule_name || row.original.rule_id || '-'}</span>
        ),
        size: 120,
      },
      {
        accessorKey: 'excerpt',
        id: 'excerpt',
        header: t('Excerpt'),
        cell: ({ row }) => (
          <span className='font-mono text-xs'>
            {row.original.excerpt || '-'}
          </span>
        ),
        size: 140,
      },
      {
        accessorKey: 'category',
        id: 'category',
        header: t('Category'),
        cell: ({ row }) => (
          <span className='font-mono text-xs'>
            {row.original.category || '-'}
          </span>
        ),
        size: 110,
      },
      {
        accessorKey: 'ip',
        id: 'ip',
        header: t('IP'),
        cell: ({ row }) => (
          <span className='text-muted-foreground font-mono text-xs'>
            {row.original.ip || '-'}
          </span>
        ),
        size: 120,
      },
      {
        id: 'actions',
        header: () => t('Actions'),
        cell: ({ row }) => (
          <Button
            variant='ghost'
            size='sm'
            onClick={() => onView(row.original)}
          >
            {t('View')}
          </Button>
        ),
        meta: { pinned: 'right' as const },
        size: 80,
      },
    ],
    [t, onView]
  )
}
