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
import { Building2 } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { IconBadge } from '@/components/ui/icon-badge'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import type { AnalyticsDepartment } from '@/features/dashboard/types'
import { formatNumber, formatPercent, formatQuota } from '@/lib/format'

interface DeptComparisonProps {
  departments?: AnalyticsDepartment[]
  loading?: boolean
}

export function DeptComparison(props: DeptComparisonProps) {
  const { t } = useTranslation()
  const departments = props.departments ?? []

  let body: ReactNode
  if (props.loading) {
    body = (
      <div className='p-3 sm:p-5'>
        <Skeleton className='h-40 w-full' />
      </div>
    )
  } else if (departments.length === 0) {
    body = (
      <EmptyState
        icon={Building2}
        title={t('No Data')}
        description={t('No department data available in the selected range')}
      />
    )
  } else {
    body = (
      <div className='overflow-x-auto'>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Department')}</TableHead>
              <TableHead className='text-right'>{t('Members')}</TableHead>
              <TableHead className='text-right'>{t('Active Users')}</TableHead>
              <TableHead className='text-right'>{t('Requests')}</TableHead>
              <TableHead className='text-right'>{t('Failure Rate')}</TableHead>
              <TableHead className='text-right'>
                {t('Net Consumption')}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {departments.map((dept) => (
              <TableRow key={dept.dept_id}>
                <TableCell className='font-medium'>
                  {dept.dept_name || dept.dept_id}
                </TableCell>
                <TableCell className='text-right'>
                  {formatNumber(dept.member_count)}
                </TableCell>
                <TableCell className='text-right'>
                  {formatNumber(dept.active_users)}
                </TableCell>
                <TableCell className='text-right'>
                  {formatNumber(dept.request_count)}
                </TableCell>
                <TableCell className='text-right'>
                  {formatPercent(dept.fail_rate * 100)}
                </TableCell>
                <TableCell className='text-right'>
                  {formatQuota(dept.quota)}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    )
  }

  return (
    <div className='overflow-hidden rounded-lg border'>
      <div className='flex w-full items-center gap-2 border-b px-3 py-2 sm:px-5 sm:py-3'>
        <IconBadge tone='info' size='sm'>
          <Building2 />
        </IconBadge>
        <div className='text-sm font-semibold'>
          {t('Department Comparison')}
        </div>
        <span className='text-muted-foreground text-xs'>
          {t('Members are attributed to their primary department')}
        </span>
      </div>
      {body}
    </div>
  )
}
