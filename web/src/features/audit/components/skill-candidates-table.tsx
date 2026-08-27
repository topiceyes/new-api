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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { ColumnDef } from '@tanstack/react-table'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DataTablePage, TruncatedCell, useDataTable } from '@/components/data-table'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'

import {
  approveSkillCandidate,
  listSkillCandidates,
  rejectSkillCandidate,
} from '../api'
import type { SkillCandidate } from '../types'

function formatTime(ts: number): string {
  if (!ts) return '-'
  return new Date(ts * 1000).toLocaleString()
}

const statusVariant: Record<string, 'info' | 'warning' | 'success' | 'neutral'> =
  {
    pending: 'warning',
    published: 'success',
    rejected: 'neutral',
  }

export function SkillCandidatesTable() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [pagination, setPagination] = useState({ pageIndex: 0, pageSize: 20 })
  const [keywordInput, setKeywordInput] = useState('')
  const [keyword, setKeyword] = useState('')
  const [approveTarget, setApproveTarget] = useState<SkillCandidate | undefined>()
  const [approveForm, setApproveForm] = useState({
    title: '',
    category: '',
    description: '',
  })

  const { data, isLoading, isFetching } = useQuery({
    queryKey: ['audit-skill-candidates', pagination.pageIndex, pagination.pageSize, keyword],
    queryFn: async () => {
      const result = await listSkillCandidates(
        pagination.pageIndex + 1,
        pagination.pageSize,
        'pending',
        keyword
      )
      if (!result.success) {
        toast.error(result.message || t('Failed to load skill candidates'))
        return { items: [], total: 0 }
      }
      return result.data
    },
    placeholderData: (prev) => prev,
  })

  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: ['audit-skill-candidates'] })

  const approveMutation = useMutation({
    mutationFn: () =>
      approveSkillCandidate(approveTarget!.id, {
        title: approveForm.title || undefined,
        category: approveForm.category || undefined,
        description: approveForm.description || undefined,
      }),
    onSuccess: (result) => {
      if (result.success) {
        toast.success(t('Published to skill library'))
        setApproveTarget(undefined)
        invalidate()
      } else {
        toast.error(result.message || t('Operation failed'))
      }
    },
    onError: () => toast.error(t('Operation failed')),
  })

  const rejectMutation = useMutation({
    mutationFn: (id: number) => rejectSkillCandidate(id),
    onSuccess: (result) => {
      if (result.success) {
        toast.success(t('Rejected'))
        invalidate()
      } else {
        toast.error(result.message || t('Operation failed'))
      }
    },
    onError: () => toast.error(t('Operation failed')),
  })

  const columns = useMemo(
    (): ColumnDef<SkillCandidate>[] => [
      {
        accessorKey: 'title',
        id: 'title',
        header: t('Skill Title'),
        cell: ({ row }) => <span className='font-medium'>{row.original.title}</span>,
        size: 160,
      },
      {
        accessorKey: 'category',
        id: 'category',
        header: t('Category'),
        cell: ({ row }) => (
          <span className='font-mono text-xs'>{row.original.category || '-'}</span>
        ),
        size: 110,
      },
      {
        accessorKey: 'occurrence_count',
        id: 'occurrence_count',
        header: t('Occurrences'),
        cell: ({ row }) => row.original.occurrence_count,
        size: 90,
      },
      {
        accessorKey: 'user_count',
        id: 'user_count',
        header: t('Users'),
        cell: ({ row }) => row.original.user_count,
        size: 70,
      },
      {
        accessorKey: 'sample_prompt',
        id: 'sample_prompt',
        header: t('Sample Prompt'),
        cell: ({ row }) => (
          <TruncatedCell>{row.original.sample_prompt || '-'}</TruncatedCell>
        ),
        size: 240,
      },
      {
        accessorKey: 'updated_at',
        id: 'updated_at',
        header: t('Last Seen'),
        cell: ({ row }) => (
          <span className='text-muted-foreground'>
            {formatTime(row.original.updated_at)}
          </span>
        ),
        size: 150,
      },
      {
        id: 'actions',
        header: () => t('Actions'),
        cell: ({ row }) => (
          <div className='flex gap-1'>
            <Button
              variant='ghost'
              size='sm'
              onClick={() => {
                setApproveTarget(row.original)
                setApproveForm({
                  title: row.original.title,
                  category: row.original.category,
                  description: '',
                })
              }}
            >
              {t('Publish')}
            </Button>
            <Button
              variant='ghost'
              size='sm'
              onClick={() => rejectMutation.mutate(row.original.id)}
            >
              {t('Reject')}
            </Button>
          </div>
        ),
        meta: { pinned: 'right' as const },
        size: 140,
      },
    ],
    [t, rejectMutation]
  )

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
      <Input
        className='w-[220px]'
        placeholder={t('Search title / category')}
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
    <>
      <DataTablePage
        table={table}
        columns={columns}
        isLoading={isLoading || (isFetching && !data)}
        isFetching={isFetching}
        emptyTitle={t('No Pending Candidates')}
        emptyDescription={t(
          'Skill candidates appear here after the classification task analyzes stored prompts.'
        )}
        skeletonKeyPrefix='skill-candidate-skeleton'
        applyHeaderSize
        toolbar={toolbar}
      />

      <Dialog
        open={approveTarget !== undefined}
        onOpenChange={(open) => !open && setApproveTarget(undefined)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('Publish Skill')}</DialogTitle>
          </DialogHeader>
          <div className='grid gap-4'>
            <div className='grid gap-2'>
              <Label>{t('Skill Title')}</Label>
              <Input
                value={approveForm.title}
                onChange={(event) =>
                  setApproveForm((prev) => ({ ...prev, title: event.target.value }))
                }
              />
            </div>
            <div className='grid gap-2'>
              <Label>{t('Category')}</Label>
              <Input
                value={approveForm.category}
                onChange={(event) =>
                  setApproveForm((prev) => ({ ...prev, category: event.target.value }))
                }
              />
            </div>
            <div className='grid gap-2'>
              <Label>{t('Description')}</Label>
              <Textarea
                rows={3}
                value={approveForm.description}
                onChange={(event) =>
                  setApproveForm((prev) => ({
                    ...prev,
                    description: event.target.value,
                  }))
                }
              />
            </div>
            {approveTarget?.sample_prompt && (
              <div className='grid gap-2'>
                <Label>{t('Sample Prompt')}</Label>
                <pre className='bg-muted max-h-40 overflow-auto rounded p-2 text-xs whitespace-pre-wrap'>
                  {approveTarget.sample_prompt}
                </pre>
              </div>
            )}
          </div>
          <DialogFooter>
            <Button variant='outline' onClick={() => setApproveTarget(undefined)}>
              {t('Cancel')}
            </Button>
            <Button
              disabled={approveMutation.isPending || approveForm.title.trim() === ''}
              onClick={() => approveMutation.mutate()}
            >
              {t('Publish')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}

export function statusBadgeFor(status: string) {
  return (
    <StatusBadge
      label={status}
      variant={statusVariant[status] ?? 'neutral'}
      copyable={false}
      className='-ml-1.5'
    />
  )
}
