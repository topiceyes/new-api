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
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

interface PagerProps {
  page: number
  pageSize: number
  total: number
  onPageChange: (page: number) => void
}

// 简单前后翻页器,与 wallet 账单记录弹窗中的分页样式一致。
export function Pager(props: PagerProps) {
  const { t } = useTranslation()
  const totalPages = Math.max(1, Math.ceil(props.total / props.pageSize))

  if (props.total === 0) return null

  return (
    <div className='flex items-center justify-between gap-3 pt-2'>
      <div className='text-muted-foreground text-xs'>
        {t('Showing')} {(props.page - 1) * props.pageSize + 1}-
        {Math.min(props.page * props.pageSize, props.total)} {t('of')}{' '}
        {props.total}
      </div>
      <div className='flex items-center gap-2'>
        <Button
          variant='outline'
          size='sm'
          onClick={() => props.onPageChange(props.page - 1)}
          disabled={props.page <= 1}
          className='h-8 w-8 p-0'
          aria-label={t('Previous page')}
        >
          <ChevronLeft className='h-4 w-4' aria-hidden='true' />
        </Button>
        <div className='text-muted-foreground flex items-center gap-1 text-sm'>
          <span className='font-medium'>{props.page}</span>
          <span>/</span>
          <span>{totalPages}</span>
        </div>
        <Button
          variant='outline'
          size='sm'
          onClick={() => props.onPageChange(props.page + 1)}
          disabled={props.page >= totalPages}
          className='h-8 w-8 p-0'
          aria-label={t('Next page')}
        >
          <ChevronRight className='h-4 w-4' aria-hidden='true' />
        </Button>
      </div>
    </div>
  )
}
