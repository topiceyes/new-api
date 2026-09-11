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
import { Search } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
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

import { listAdminRechargeRequests } from '../api'
import { CATEGORY_LABEL_KEY } from '../constants'
import type { RechargeCategory, RechargeRequestStatus } from '../types'
import { RechargeStatusBadge } from './badges'
import { Pager } from './pager'
import { RequestDetailDialog } from './request-detail-dialog'

const PAGE_SIZE = 10

const SKELETON_KEYS = ['sk-1', 'sk-2', 'sk-3', 'sk-4']

const ALL = '__all__'

export function AdminRequestsTable() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [status, setStatus] = useState<RechargeRequestStatus | ''>('')
  const [category, setCategory] = useState<RechargeCategory | ''>('')
  const [keywordInput, setKeywordInput] = useState('')
  const [keyword, setKeyword] = useState('')
  const [detailId, setDetailId] = useState<number | null>(null)

  const listQuery = useQuery({
    queryKey: ['recharge-admin-requests', status, category, keyword, page],
    queryFn: async () => {
      const res = await listAdminRechargeRequests({
        status,
        category,
        keyword,
        page,
        pageSize: PAGE_SIZE,
      })
      if (!res.success) throw new Error(res.message)
      return res.data
    },
    placeholderData: (prev) => prev,
  })

  const items = listQuery.data?.items ?? []
  const total = listQuery.data?.total ?? 0

  const applyKeyword = () => {
    setKeyword(keywordInput.trim())
    setPage(1)
  }

  let tableContent: ReactNode
  if (listQuery.isLoading) {
    tableContent = (
      <div className='space-y-2'>
        {SKELETON_KEYS.map((key) => (
          <Skeleton key={key} className='h-9 w-full rounded-md' />
        ))}
      </div>
    )
  } else if (listQuery.isError) {
    tableContent = (
      <ErrorState
        title={t('Failed to load recharge requests.')}
        onRetry={() => void listQuery.refetch()}
        className='min-h-[200px]'
      />
    )
  } else if (items.length === 0) {
    tableContent = (
      <div className='text-muted-foreground rounded-md border border-dashed px-4 py-10 text-center text-sm'>
        {t('No recharge requests found.')}
      </div>
    )
  } else {
    tableContent = (
      <>
        <div className='overflow-x-auto rounded-md border'>
          <Table className='min-w-[900px]'>
            <TableHeader>
              <TableRow className='bg-muted/40 hover:bg-muted/40'>
                <TableHead className='h-9 w-[70px] px-4 text-xs'>
                  {t('ID')}
                </TableHead>
                <TableHead className='h-9 w-[120px] text-xs'>
                  {t('Applicant')}
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
                  <TableCell className='py-3 align-middle text-sm'>
                    {item.display_name || item.username}
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
    <div className='space-y-3'>
      <div className='flex flex-wrap items-center gap-2'>
        <Select
          value={status === '' ? ALL : status}
          onValueChange={(value) => {
            setStatus(value === ALL ? '' : (value as RechargeRequestStatus))
            setPage(1)
          }}
        >
          <SelectTrigger className='h-9 w-36'>
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            <SelectGroup>
              <SelectItem value={ALL}>{t('All Statuses')}</SelectItem>
              <SelectItem value='pending'>{t('Pending')}</SelectItem>
              <SelectItem value='approved'>{t('Approved')}</SelectItem>
              <SelectItem value='rejected'>{t('Rejected')}</SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>

        <Select
          value={category === '' ? ALL : category}
          onValueChange={(value) => {
            setCategory(value === ALL ? '' : (value as RechargeCategory))
            setPage(1)
          }}
        >
          <SelectTrigger className='h-9 w-40'>
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            <SelectGroup>
              <SelectItem value={ALL}>{t('All Categories')}</SelectItem>
              <SelectItem value='project_delivery'>
                {t(CATEGORY_LABEL_KEY.project_delivery)}
              </SelectItem>
              <SelectItem value='tech_research'>
                {t(CATEGORY_LABEL_KEY.tech_research)}
              </SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>

        <div className='relative min-w-[200px] flex-1'>
          <Search
            className='text-muted-foreground absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2'
            aria-hidden='true'
          />
          <Input
            placeholder={t('Search by name or username...')}
            value={keywordInput}
            onChange={(e) => setKeywordInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') applyKeyword()
            }}
            className='h-9 pl-10'
          />
        </div>
        <Button variant='outline' size='sm' onClick={applyKeyword}>
          {t('Search')}
        </Button>
      </div>

      {tableContent}

      <RequestDetailDialog
        requestId={detailId}
        open={detailId !== null}
        onOpenChange={(open) => {
          if (!open) setDetailId(null)
        }}
        admin
      />
    </div>
  )
}
