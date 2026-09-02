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
import { getRouteApi } from '@tanstack/react-router'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { DataTablePage, useDataTable } from '@/components/data-table'
import { useMediaQuery } from '@/hooks'
import { useTableUrlState } from '@/hooks/use-table-url-state'

import { getAnalyticsUserStatusOptions } from '../constants'
import type { AnalyticsUserTableEntry } from '../types'
import { useAnalyticsUserColumns } from './user-table-columns'

const route = getRouteApi('/_authenticated/analytics/users')

type AnalyticsUserTableProps = {
  entries: AnalyticsUserTableEntry[]
  isLoading?: boolean
  isFetching?: boolean
}

export function AnalyticsUserTable(props: AnalyticsUserTableProps) {
  const { entries, isLoading = false, isFetching = false } = props
  const { t } = useTranslation()
  const isMobile = useMediaQuery('(max-width: 640px)')

  const {
    globalFilter,
    onGlobalFilterChange,
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search: route.useSearch(),
    navigate: route.useNavigate(),
    pagination: { defaultPage: 1, defaultPageSize: isMobile ? 10 : 20 },
    globalFilter: { enabled: true, key: 'filter' },
    columnFilters: [
      { columnId: 'status', searchKey: 'status', type: 'array' },
      { columnId: 'dept_name', searchKey: 'dept', type: 'array' },
      { columnId: 'top_model', searchKey: 'model', type: 'array' },
    ],
  })

  // Department column/filter only make sense when org sync provides dept names.
  const hasDept = useMemo(() => entries.some((e) => e.dept_name), [entries])
  const columns = useAnalyticsUserColumns({ showDept: hasDept })

  const deptOptions = useMemo(
    () =>
      [...new Set(entries.map((e) => e.dept_name).filter(Boolean))]
        .sort()
        .map((dept) => ({ value: dept, label: dept })),
    [entries]
  )
  const modelOptions = useMemo(
    () =>
      [...new Set(entries.map((e) => e.top_model).filter(Boolean))]
        .sort()
        .map((model) => ({ value: model, label: model })),
    [entries]
  )

  const { table } = useDataTable({
    data: entries,
    columns,
    getRowId: (row) => row.member_key,
    columnFilters,
    globalFilter,
    pagination,
    onColumnFiltersChange,
    onGlobalFilterChange,
    onPaginationChange,
    globalFilterFn: (row, _columnId, filterValue) => {
      const searchValue = String(filterValue).toLowerCase()
      const fields = [
        row.original.display_name,
        row.original.username,
        row.original.top_model,
      ]
      return fields.some((field) =>
        String(field || '')
          .toLowerCase()
          .includes(searchValue)
      )
    },
    ensurePageInRange,
  })

  const filters = [
    {
      columnId: 'status',
      title: t('Status'),
      options: getAnalyticsUserStatusOptions(t),
    },
    ...(hasDept
      ? [{ columnId: 'dept_name', title: t('Department'), options: deptOptions }]
      : []),
    ...(modelOptions.length > 1
      ? [{ columnId: 'top_model', title: t('Top Model'), options: modelOptions }]
      : []),
  ]

  return (
    <DataTablePage
      table={table}
      columns={columns}
      isLoading={isLoading}
      isFetching={isFetching}
      emptyTitle={t('No Users Found')}
      emptyDescription={t(
        'No users in scope. Try adjusting the time range or filters.'
      )}
      skeletonKeyPrefix='analytics-users-skeleton'
      applyHeaderSize
      toolbarProps={{
        searchPlaceholder: t('Search by name or model...'),
        searchDebounceMs: 300,
        filters,
      }}
    />
  )
}
