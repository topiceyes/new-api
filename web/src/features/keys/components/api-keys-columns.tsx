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
import type { ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { Checkbox } from '@/components/ui/checkbox'
import { Progress } from '@/components/ui/progress'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { useMediaQuery } from '@/hooks'
import { toIntlLocale } from '@/i18n/languages'
import { getUserGroups } from '@/lib/api'
import dayjs from '@/lib/dayjs'
import { formatNumber, formatQuota, formatTimestampRelative, formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { API_KEY_STATUSES } from '../constants'
import type { ApiKey } from '../types'
import { ApiKeyGroupCell } from './api-key-group-cell'
import { ApiKeyTimestampCell } from './api-key-timestamp-cell'
import {
  ApiKeyCell,
  IpRestrictionsCell,
  ModelLimitsCell,
  UnlimitedQuotaBadge,
} from './api-keys-cells'
import { DataTableRowActions } from './data-table-row-actions'

function getQuotaProgressColor(percentage: number): string {
  if (percentage <= 10) return '[&_[data-slot=progress-indicator]]:bg-rose-500'
  if (percentage <= 30) return '[&_[data-slot=progress-indicator]]:bg-amber-500'
  return '[&_[data-slot=progress-indicator]]:bg-emerald-500'
}

function useGroupRatios(): Record<string, number | string> {
  const { data } = useQuery({
    queryKey: ['user-groups'],
    queryFn: getUserGroups,
    staleTime: 0,
    select: (res) => {
      if (!res.success || !res.data) return {}
      const ratios: Record<string, number | string> = {}
      for (const [group, info] of Object.entries(res.data)) {
        if (typeof info.ratio === 'number' || typeof info.ratio === 'string') {
          ratios[group] = info.ratio
        }
      }
      return ratios
    },
  })

  return data ?? {}
}

export function useApiKeysColumns(now: number): ColumnDef<ApiKey>[] {
  const { t, i18n } = useTranslation()
  const groupRatios = useGroupRatios()
  const shouldReduceMotion = useMediaQuery('(prefers-reduced-motion: reduce)')
  const isRoot = useAuthStore((state) => state.auth.user?.role === 100)
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const justNowLabel = t('Just now')
  const staleAccessThreshold = dayjs(now).subtract(3, 'month').valueOf()
  return [
    {
      id: 'select',
      header: ({ table }) => (
        <Checkbox
          checked={table.getIsAllPageRowsSelected()}
          indeterminate={table.getIsSomePageRowsSelected()}
          onCheckedChange={(value) => table.toggleAllPageRowsSelected(!!value)}
          aria-label='Select all'
          className='translate-y-[2px]'
        />
      ),
      cell: ({ row }) => (
        <Checkbox
          checked={row.getIsSelected()}
          onCheckedChange={(value) => row.toggleSelected(!!value)}
          aria-label='Select row'
          className='translate-y-[2px]'
        />
      ),
      enableSorting: false,
      enableHiding: false,
      size: 40,
    },
    {
      accessorKey: 'name',
      header: t('Name'),
      cell: ({ row }) => (
        <span className='font-medium'>{row.getValue('name')}</span>
      ),
      size: 180,
      meta: { mobileTitle: true },
    },
    // root 全员视图: key 的归属用户
    ...(isRoot
      ? [
          {
            accessorKey: 'owner_name',
            header: t('User'),
            cell: ({ row }) => {
              const owner = row.original.owner_name
              return (
                <span className='text-muted-foreground text-sm'>
                  {owner || `#${row.original.user_id ?? '-'}`}
                </span>
              )
            },
            enableSorting: false,
            size: 120,
            meta: { mobileOrder: 15 },
          } satisfies ColumnDef<ApiKey>,
        ]
      : []),
    {
      accessorKey: 'status',
      header: t('Status'),
      cell: ({ row }) => {
        const statusConfig = API_KEY_STATUSES[row.getValue('status') as number]
        if (!statusConfig) return null
        return (
          <StatusBadge
            label={t(statusConfig.label)}
            variant={statusConfig.variant}
            copyable={false}
            className='-ml-1.5'
          />
        )
      },
      filterFn: (row, id, value) => value.includes(String(row.getValue(id))),
      size: 120,
      meta: { mobileBadge: true },
    },
    {
      id: 'key',
      accessorKey: 'key',
      header: t('API Key'),
      cell: ({ row }) => <ApiKeyCell apiKey={row.original} />,
      enableSorting: false,
      size: 260,
    },
    {
      id: 'quota',
      accessorKey: 'remain_quota',
      header: t('Quota'),
      cell: ({ row }) => {
        const apiKey = row.original
        if (apiKey.unlimited_quota) {
          return <UnlimitedQuotaBadge used={apiKey.used_quota} />
        }

        const used = apiKey.used_quota
        const remaining = apiKey.remain_quota
        const total = used + remaining
        const percentage = total > 0 ? (remaining / total) * 100 : 0

        return (
          <Tooltip>
            <TooltipTrigger render={<div className='w-[150px] space-y-1' />}>
              <div className='flex justify-between text-xs'>
                <span className='font-medium tabular-nums'>
                  {formatQuota(remaining)}
                </span>
                <span className='text-muted-foreground tabular-nums'>
                  {formatQuota(total)}
                </span>
              </div>
              <Progress
                value={percentage}
                className={cn('h-1.5', getQuotaProgressColor(percentage))}
              />
            </TooltipTrigger>
            <TooltipContent>
              <div className='space-y-1 text-xs'>
                <div>
                  {t('Used:')} {formatQuota(used)}
                </div>
                <div>
                  {t('Remaining:')} {formatQuota(remaining)} (
                  {percentage.toFixed(1)}%)
                </div>
                <div>
                  {t('Total:')} {formatQuota(total)}
                </div>
              </div>
            </TooltipContent>
          </Tooltip>
        )
      },
      size: 170,
    },
    {
      // 累计消耗: tokens(consume 日志求和) + 金额(used_quota)
      id: 'consumption',
      accessorKey: 'used_tokens',
      header: t('Consumption'),
      cell: ({ row }) => {
        const apiKey = row.original
        return (
          <div className='flex flex-col'>
            <span className='text-sm tabular-nums'>
              {formatNumber(apiKey.used_tokens ?? 0)} {t('tokens')}
            </span>
            <span className='text-muted-foreground text-xs tabular-nums'>
              {formatQuota(apiKey.used_quota)}
            </span>
          </div>
        )
      },
      enableSorting: false,
      size: 130,
      meta: { mobileOrder: 25 },
    },
    {
      accessorKey: 'last_user_agent',
      header: t('User-Agent'),
      cell: ({ row }) => {
        const ua = row.original.last_user_agent
        if (!ua) {
          return <span className='text-muted-foreground text-sm'>-</span>
        }
        return (
          <Tooltip>
            <TooltipTrigger render={<div className='max-w-[180px]' />}>
              <span className='text-muted-foreground block truncate text-xs'>
                {ua}
              </span>
            </TooltipTrigger>
            <TooltipContent>
              <p className='max-w-[320px] text-xs break-all'>{ua}</p>
            </TooltipContent>
          </Tooltip>
        )
      },
      enableSorting: false,
      size: 190,
      meta: { mobileHidden: true },
    },
    {
      accessorKey: 'last_ip',
      header: t('Last IP'),
      cell: ({ row }) => {
        const ip = row.original.last_ip
        return (
          <span className='font-mono text-sm'>{ip || '-'}</span>
        )
      },
      enableSorting: false,
      size: 130,
      meta: { mobileHidden: true },
    },
    {
      accessorKey: 'group',
      header: t('Group'),
      cell: ({ row }) => {
        const apiKey = row.original
        const group = row.getValue('group') as string
        return (
          <ApiKeyGroupCell
            group={group}
            ratio={groupRatios[group]}
            crossGroupRetry={apiKey.cross_group_retry}
            shouldReduceMotion={shouldReduceMotion}
          />
        )
      },
      size: 220,
      meta: { mobileHidden: true },
    },
    {
      id: 'model_limits',
      accessorKey: 'model_limits',
      header: t('Models'),
      cell: ({ row }) => <ModelLimitsCell apiKey={row.original} />,
      enableSorting: false,
      size: 160,
      meta: { mobileHidden: true },
    },
    {
      id: 'allow_ips',
      accessorKey: 'allow_ips',
      header: t('IP Restriction'),
      cell: ({ row }) => <IpRestrictionsCell apiKey={row.original} />,
      enableSorting: false,
      size: 160,
      meta: { mobileHidden: true },
    },
    {
      accessorKey: 'created_time',
      header: t('Created'),
      cell: ({ row }) => (
        <ApiKeyTimestampCell
          timestamp={row.getValue('created_time')}
          now={now}
          locale={locale}
          justNowLabel={justNowLabel}
          className='text-muted-foreground'
        />
      ),
      size: 180,
      meta: { mobileHidden: true },
    },
    {
      accessorKey: 'accessed_time',
      header: t('Last Used'),
      cell: ({ row }) => {
        const accessedTime = row.getValue('accessed_time') as number
        const isStale =
          accessedTime > 0 && accessedTime * 1000 < staleAccessThreshold

        // 具体时间点(相对时间在悬停提示里)
        if (!accessedTime || accessedTime <= 0) {
          return <span className='text-muted-foreground text-xs'>-</span>
        }
        return (
          <Tooltip>
            <TooltipTrigger
              render={
                <time
                  dateTime={new Date(accessedTime * 1000).toISOString()}
                  tabIndex={0}
                  className={cn(
                    'block truncate font-mono text-xs tabular-nums',
                    isStale ? 'text-warning' : 'text-muted-foreground'
                  )}
                />
              }
            >
              {formatTimestampToDate(accessedTime)}
            </TooltipTrigger>
            <TooltipContent>
              <span className='font-mono tabular-nums'>
                {accessedTime * 1000 > now - 60_000
                  ? justNowLabel
                  : formatTimestampRelative(accessedTime, 'seconds', locale)}
              </span>
            </TooltipContent>
          </Tooltip>
        )
      },
      size: 180,
      meta: { mobileHidden: true },
    },
    {
      accessorKey: 'expired_time',
      header: t('Expires'),
      cell: ({ row }) => {
        const expiredTime = row.getValue('expired_time') as number
        if (expiredTime === -1) {
          return (
            <StatusBadge
              label={t('Never')}
              variant='neutral'
              copyable={false}
              className='-ml-1.5'
            />
          )
        }
        const isExpired = expiredTime * 1000 < now
        return (
          <ApiKeyTimestampCell
            timestamp={expiredTime}
            now={now}
            locale={locale}
            justNowLabel={justNowLabel}
            className={cn(
              isExpired ? 'text-destructive' : 'text-muted-foreground'
            )}
          />
        )
      },
      size: 180,
      meta: { mobileHidden: true },
    },
    {
      id: 'actions',
      header: () => t('Actions'),
      cell: ({ row }) => <DataTableRowActions row={row} />,
      meta: { pinned: 'right' as const },
    },
  ]
}
