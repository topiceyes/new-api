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
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DataTablePage, useDataTable } from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import { listAuditEvents } from '../api'
import type { AuditEvent, AuditEventFilters } from '../types'
import { AUDIT_EVENT_TYPES, AUDIT_SEVERITIES } from '../types'
import { useAuditEventColumns } from './audit-event-columns'

const ALL_VALUE = '__all__'

interface Props {
  onView: (event: AuditEvent) => void
}

export function AuditEventsTable({ onView }: Props) {
  const { t } = useTranslation()
  const [pagination, setPagination] = useState({ pageIndex: 0, pageSize: 20 })
  const [eventType, setEventType] = useState<string>(ALL_VALUE)
  const [severity, setSeverity] = useState<string>(ALL_VALUE)
  const [keywordInput, setKeywordInput] = useState('')
  const [keyword, setKeyword] = useState('')

  const filters: AuditEventFilters = {
    ...(eventType !== ALL_VALUE ? { event_type: eventType } : {}),
    ...(severity !== ALL_VALUE ? { severity } : {}),
    ...(keyword ? { keyword } : {}),
  }

  const { data, isLoading, isFetching } = useQuery({
    queryKey: [
      'audit-events',
      pagination.pageIndex,
      pagination.pageSize,
      eventType,
      severity,
      keyword,
    ],
    queryFn: async () => {
      const result = await listAuditEvents(
        pagination.pageIndex + 1,
        pagination.pageSize,
        filters
      )
      if (!result.success) {
        toast.error(result.message || t('Failed to load audit events'))
        return { items: [], total: 0 }
      }
      return result.data
    },
    placeholderData: (prev) => prev,
  })

  const columns = useAuditEventColumns(onView)

  const { table } = useDataTable({
    data: data?.items ?? [],
    columns,
    pagination,
    onPaginationChange: setPagination,
    manualPagination: true,
    manualFiltering: true,
    totalCount: data?.total ?? 0,
    enableRowSelection: false,
  })

  const applyKeyword = () => {
    setKeyword(keywordInput.trim())
    setPagination((prev) => ({ ...prev, pageIndex: 0 }))
  }

  const toolbar = (
    <div className='flex flex-wrap items-center gap-2'>
      <Select
        value={eventType}
        onValueChange={(value) => {
          // Base UI 清空时回调可能给 null,回落到"全部"
          setEventType(value ?? ALL_VALUE)
          setPagination((prev) => ({ ...prev, pageIndex: 0 }))
        }}
        items={[
          { value: ALL_VALUE, label: t('All Types') },
          ...AUDIT_EVENT_TYPES.map((type) => ({ value: type, label: type })),
        ]}
      >
        <SelectTrigger className='w-[180px]'>
          <SelectValue placeholder={t('Event Type')} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ALL_VALUE}>{t('All Types')}</SelectItem>
          {AUDIT_EVENT_TYPES.map((type) => (
            <SelectItem key={type} value={type}>
              {type}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Select
        value={severity}
        onValueChange={(value) => {
          setSeverity(value ?? ALL_VALUE)
          setPagination((prev) => ({ ...prev, pageIndex: 0 }))
        }}
        items={[
          { value: ALL_VALUE, label: t('All Severities') },
          ...AUDIT_SEVERITIES.map((item) => ({ value: item, label: t(item) })),
        ]}
      >
        <SelectTrigger className='w-[140px]'>
          <SelectValue placeholder={t('Severity')} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ALL_VALUE}>{t('All Severities')}</SelectItem>
          {AUDIT_SEVERITIES.map((item) => (
            <SelectItem key={item} value={item}>
              {t(item)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Input
        className='w-[220px]'
        placeholder={t('Search username / rule / model')}
        value={keywordInput}
        onChange={(event) => setKeywordInput(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === 'Enter') applyKeyword()
        }}
      />
      <Button variant='outline' onClick={applyKeyword}>
        {t('Search')}
      </Button>
    </div>
  )

  return (
    <DataTablePage
      table={table}
      columns={columns}
      isLoading={isLoading || (isFetching && !data)}
      isFetching={isFetching}
      emptyTitle={t('No Audit Events')}
      emptyDescription={t(
        'Audit events will appear here once security rules are triggered.'
      )}
      skeletonKeyPrefix='audit-event-skeleton'
      applyHeaderSize
      toolbar={toolbar}
    />
  )
}
