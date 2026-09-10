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
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatQuotaWithCurrency } from '@/lib/currency'
import { formatTimestampToDate } from '@/lib/format'

import { listMyRechargeRequests } from '../api'
import { CATEGORY_LABEL_KEY } from '../constants'
import { RechargeStatusBadge } from './badges'
import { Pager } from './pager'
import { RequestDetailDialog } from './request-detail-dialog'

const PAGE_SIZE = 10

const SKELETON_KEYS = ['sk-1', 'sk-2', 'sk-3', 'sk-4']

export function MyRequestsTable() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [detailId, setDetailId] = useState<number | null>(null)

  const listQuery = useQuery({
    queryKey: ['recharge-my-requests', page],
    queryFn: async () => {
      const res = await listMyRechargeRequests(page, PAGE_SIZE)
      if (!res.success) throw new Error(res.message)
      return res.data
    },
    placeholderData: (prev) => prev,
  })

  const items = listQuery.data?.items ?? []
  const total = listQuery.data?.total ?? 0

  let content: ReactNode
  if (listQuery.isLoading) {
    content = (
      <div className='space-y-2'>
        {SKELETON_KEYS.map((key) => (
          <Skeleton key={key} className='h-9 w-full rounded-md' />
        ))}
      </div>
    )
  } else if (listQuery.isError) {
    content = (
      <ErrorState
        title={t('Failed to load recharge requests.')}
        onRetry={() => void listQuery.refetch()}
        className='min-h-[200px]'
      />
    )
  } else if (items.length === 0) {
    content = (
      <div className='text-muted-foreground rounded-md border border-dashed px-4 py-10 text-center text-sm'>
        {t('No recharge requests yet.')}
      </div>
    )
  } else {
    content = (
      <>
        <div className='overflow-x-auto rounded-md border'>
          <Table className='min-w-[860px]'>
            <TableHeader>
              <TableRow className='bg-muted/40 hover:bg-muted/40'>
                <TableHead className='h-9 w-[70px] px-4 text-xs'>
                  {t('ID')}
                </TableHead>
                <TableHead className='h-9 w-[130px] text-xs'>
                  {t('Category')}
                </TableHead>
                <TableHead className='h-9 w-[110px] text-xs'>
                  {t('Amount')}
                </TableHead>
                <TableHead className='h-9 w-[100px] text-xs'>
                  {t('Status')}
                </TableHead>
                <TableHead className='h-9 w-[100px] text-xs'>
                  {t('Progress')}
                </TableHead>
                <TableHead className='h-9 w-[170px] text-xs'>
                  {t('Created At')}
                </TableHead>
                <TableHead className='h-9 min-w-[160px] text-xs'>
                  {t('Reject Reason')}
                </TableHead>
                <TableHead className='h-9 w-[90px] pr-4 text-xs'>
                  {t('Actions')}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((item) => (
                <TableRow key={item.id} className='hover:bg-muted/30'>
                  <TableCell className='px-4 py-3 align-middle font-mono text-xs'>
                    {item.id}
                  </TableCell>
                  <TableCell className='py-3 align-middle'>
                    <Badge variant='outline'>
                      {t(CATEGORY_LABEL_KEY[item.category])}
                    </Badge>
                  </TableCell>
                  <TableCell className='py-3 align-middle text-sm'>
                    {formatQuotaWithCurrency(item.quota)}
                  </TableCell>
                  <TableCell className='py-3 align-middle'>
                    <RechargeStatusBadge status={item.status} />
                  </TableCell>
                  <TableCell className='text-muted-foreground py-3 align-middle text-xs tabular-nums'>
                    {item.status === 'pending'
                      ? `${item.current_level}/${item.total_levels}`
                      : '-'}
                  </TableCell>
                  <TableCell className='text-muted-foreground py-3 align-middle text-xs whitespace-nowrap'>
                    {formatTimestampToDate(item.create_time)}
                  </TableCell>
                  <TableCell
                    className='text-muted-foreground max-w-[220px] truncate py-3 align-middle text-xs'
                    title={item.reject_reason || undefined}
                  >
                    {item.status === 'rejected' && item.reject_reason
                      ? item.reject_reason
                      : '-'}
                  </TableCell>
                  <TableCell className='py-3 pr-4 align-middle'>
                    <Button
                      variant='outline'
                      size='sm'
                      onClick={() => setDetailId(item.id)}
                    >
                      {t('Detail')}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
        <Pager
          page={page}
          pageSize={PAGE_SIZE}
          total={total}
          onPageChange={setPage}
        />
      </>
    )
  }

  return (
    <div className='space-y-2'>
      {content}

      <RequestDetailDialog
        requestId={detailId}
        open={detailId !== null}
        onOpenChange={(open) => {
          if (!open) setDetailId(null)
        }}
      />
    </div>
  )
}
