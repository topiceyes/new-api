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

import {
  DataTablePage,
  TruncatedCell,
  useDataTable,
} from '@/components/data-table'
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

import { archiveSkill, listSkills, updateSkill } from '../api'
import type { Skill } from '../types'
import { statusBadgeFor } from './skill-candidates-table'

function formatTime(ts: number): string {
  if (!ts) return '-'
  return new Date(ts * 1000).toLocaleString()
}

export function SkillsTable() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [pagination, setPagination] = useState({ pageIndex: 0, pageSize: 20 })
  const [keywordInput, setKeywordInput] = useState('')
  const [keyword, setKeyword] = useState('')
  const [editTarget, setEditTarget] = useState<Skill | undefined>()
  const [editForm, setEditForm] = useState({
    title: '',
    category: '',
    description: '',
    sample_prompt: '',
  })

  const { data, isLoading, isFetching } = useQuery({
    queryKey: ['audit-skills', pagination.pageIndex, pagination.pageSize, keyword],
    queryFn: async () => {
      const result = await listSkills(
        pagination.pageIndex + 1,
        pagination.pageSize,
        '',
        keyword
      )
      if (!result.success) {
        toast.error(result.message || t('Failed to load skills'))
        return { items: [], total: 0 }
      }
      return result.data
    },
    placeholderData: (prev) => prev,
  })

  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: ['audit-skills'] })

  const updateMutation = useMutation({
    mutationFn: () =>
      updateSkill(editTarget!.id, {
        title: editForm.title,
        category: editForm.category,
        description: editForm.description,
        sample_prompt: editForm.sample_prompt,
      }),
    onSuccess: (result) => {
      if (result.success) {
        toast.success(t('Skill updated'))
        setEditTarget(undefined)
        invalidate()
      } else {
        toast.error(result.message || t('Operation failed'))
      }
    },
    onError: () => toast.error(t('Operation failed')),
  })

  const archiveMutation = useMutation({
    mutationFn: (id: number) => archiveSkill(id),
    onSuccess: (result) => {
      if (result.success) {
        toast.success(t('Archived'))
        invalidate()
      } else {
        toast.error(result.message || t('Operation failed'))
      }
    },
    onError: () => toast.error(t('Operation failed')),
  })

  const columns = useMemo(
    (): ColumnDef<Skill>[] => [
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
        accessorKey: 'description',
        id: 'description',
        header: t('Description'),
        cell: ({ row }) => (
          <TruncatedCell>{row.original.description || '-'}</TruncatedCell>
        ),
        size: 220,
      },
      {
        accessorKey: 'status',
        id: 'status',
        header: t('Status'),
        cell: ({ row }) => statusBadgeFor(row.original.status),
        size: 100,
      },
      {
        accessorKey: 'updated_at',
        id: 'updated_at',
        header: t('Updated'),
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
                setEditTarget(row.original)
                setEditForm({
                  title: row.original.title,
                  category: row.original.category,
                  description: row.original.description,
                  sample_prompt: row.original.sample_prompt,
                })
              }}
            >
              {t('Edit')}
            </Button>
            {row.original.status !== 'archived' && (
              <Button
                variant='ghost'
                size='sm'
                onClick={() => archiveMutation.mutate(row.original.id)}
              >
                {t('Archive')}
              </Button>
            )}
          </div>
        ),
        meta: { pinned: 'right' as const },
        size: 130,
      },
    ],
    [t, archiveMutation]
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
        emptyTitle={t('No Skills Yet')}
        emptyDescription={t('Publish skill candidates to build the library.')}
        skeletonKeyPrefix='skill-skeleton'
        applyHeaderSize
        toolbar={toolbar}
      />

      <Dialog
        open={editTarget !== undefined}
        onOpenChange={(open) => !open && setEditTarget(undefined)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('Edit Skill')}</DialogTitle>
          </DialogHeader>
          <div className='grid gap-4'>
            <div className='grid gap-2'>
              <Label>{t('Skill Title')}</Label>
              <Input
                value={editForm.title}
                onChange={(event) =>
                  setEditForm((prev) => ({ ...prev, title: event.target.value }))
                }
              />
            </div>
            <div className='grid gap-2'>
              <Label>{t('Category')}</Label>
              <Input
                value={editForm.category}
                onChange={(event) =>
                  setEditForm((prev) => ({ ...prev, category: event.target.value }))
                }
              />
            </div>
            <div className='grid gap-2'>
              <Label>{t('Description')}</Label>
              <Textarea
                rows={3}
                value={editForm.description}
                onChange={(event) =>
                  setEditForm((prev) => ({
                    ...prev,
                    description: event.target.value,
                  }))
                }
              />
            </div>
            <div className='grid gap-2'>
              <Label>{t('Sample Prompt')}</Label>
              <Textarea
                rows={4}
                value={editForm.sample_prompt}
                onChange={(event) =>
                  setEditForm((prev) => ({
                    ...prev,
                    sample_prompt: event.target.value,
                  }))
                }
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant='outline' onClick={() => setEditTarget(undefined)}>
              {t('Cancel')}
            </Button>
            <Button
              disabled={updateMutation.isPending || editForm.title.trim() === ''}
              onClick={() => updateMutation.mutate()}
            >
              {t('Save')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
