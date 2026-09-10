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
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { ErrorState } from '@/components/error-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { formatQuotaWithCurrency } from '@/lib/currency'
import { formatTimestampToDate } from '@/lib/format'

import {
  approveRechargeRequest,
  listMyApprovalTasks,
  rejectRechargeRequest,
} from '../api'
import { CATEGORY_LABEL_KEY } from '../constants'
import type { ApprovalTask } from '../types'
import { RechargeDecisionBadge } from './badges'
import { Pager } from './pager'

const PAGE_SIZE = 10

const SKELETON_KEYS = ['sk-1', 'sk-2', 'sk-3']

type DecideAction = 'approve' | 'reject'

interface DecideTarget {
  task: ApprovalTask
  action: DecideAction
}

export function MyApprovalsTable() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [tab, setTab] = useState<'pending' | 'history'>('pending')
  const [page, setPage] = useState(1)
  const [target, setTarget] = useState<DecideTarget | null>(null)
  const [comment, setComment] = useState('')

  const listQuery = useQuery({
    queryKey: ['recharge-approvals', tab, page],
    queryFn: async () => {
      const res = await listMyApprovalTasks(tab === 'pending', page, PAGE_SIZE)
      if (!res.success) throw new Error(res.message)
      return res.data
    },
    placeholderData: (prev) => prev,
  })

  const decideMutation = useMutation({
    mutationFn: (input: {
      id: number
      action: DecideAction
      comment: string
    }) =>
      input.action === 'approve'
        ? approveRechargeRequest(input.id, input.comment || undefined)
        : rejectRechargeRequest(input.id, input.comment),
    onSuccess: (res) => {
      if (!res.success) {
        toast.error(res.message || t('Operation failed, please try again'))
        return
      }
      toast.success(t('Decision submitted'))
      setTarget(null)
      setComment('')
      queryClient.invalidateQueries({ queryKey: ['recharge-approvals'] })
      queryClient.invalidateQueries({ queryKey: ['recharge-config'] })
    },
    onError: (error: Error) => {
      toast.error(error.message || t('Operation failed, please try again'))
    },
  })

  const items = listQuery.data?.items ?? []
  const total = listQuery.data?.total ?? 0

  const openDecide = (task: ApprovalTask, action: DecideAction) => {
    setComment('')
    setTarget({ task, action })
  }

  const confirmDisabled =
    decideMutation.isPending ||
    (target?.action === 'reject' && comment.trim() === '')

  let content: ReactNode
  if (listQuery.isLoading) {
    content = (
      <div className='space-y-2'>
        {SKELETON_KEYS.map((key) => (
          <Skeleton key={key} className='h-24 w-full rounded-lg' />
        ))}
      </div>
    )
  } else if (listQuery.isError) {
    content = (
      <ErrorState
        title={t('Failed to load approval tasks.')}
        onRetry={() => void listQuery.refetch()}
        className='min-h-[200px]'
      />
    )
  } else if (items.length === 0) {
    content = (
      <div className='text-muted-foreground rounded-md border border-dashed px-4 py-10 text-center text-sm'>
        {tab === 'pending'
          ? t('No pending approval tasks.')
          : t('No approval history yet.')}
      </div>
    )
  } else {
    content = (
      <>
        <div className='space-y-3'>
          {items.map((task) => (
            <div
              key={`${task.id}-${task.my_level}`}
              className='rounded-lg border p-3 sm:p-4'
            >
              <div className='flex items-start justify-between gap-2'>
                <div className='min-w-0 space-y-1'>
                  <div className='flex flex-wrap items-center gap-2'>
                    <span className='text-sm font-medium'>{task.username}</span>
                    <Badge variant='outline'>
                      {t(CATEGORY_LABEL_KEY[task.category])}
                    </Badge>
                    <span className='text-sm font-semibold'>
                      {formatQuotaWithCurrency(task.quota)}
                    </span>
                  </div>
                  <div className='text-muted-foreground text-xs'>
                    {formatTimestampToDate(task.create_time)} ·{' '}
                    {t('Level {{level}}', { level: task.my_level })}
                  </div>
                </div>
                {tab === 'history' && (
                  <RechargeDecisionBadge
                    decision={
                      task.my_decision === 'rejected' ? 'rejected' : 'approved'
                    }
                  />
                )}
              </div>

              <div className='mt-2 rounded-md border p-2.5 text-sm break-words whitespace-pre-wrap'>
                {task.detail}
              </div>

              {tab === 'history' && task.my_comment && (
                <div className='text-muted-foreground mt-2 text-xs'>
                  {t('My comment')}: {task.my_comment}
                </div>
              )}

              {tab === 'pending' && task.is_actionable && (
                <div className='mt-3 flex justify-end gap-2'>
                  <Button
                    size='sm'
                    variant='outline'
                    onClick={() => openDecide(task, 'reject')}
                  >
                    {t('Reject')}
                  </Button>
                  <Button size='sm' onClick={() => openDecide(task, 'approve')}>
                    {t('Approve')}
                  </Button>
                </div>
              )}
            </div>
          ))}
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
    <div className='space-y-4'>
      <Tabs
        value={tab}
        onValueChange={(value) => {
          setTab(value as 'pending' | 'history')
          setPage(1)
        }}
      >
        <TabsList>
          <TabsTrigger value='pending'>{t('Pending')}</TabsTrigger>
          <TabsTrigger value='history'>{t('History')}</TabsTrigger>
        </TabsList>
      </Tabs>

      {content}

      <Dialog
        open={target !== null}
        onOpenChange={(open) => {
          if (!open) setTarget(null)
        }}
        title={target?.action === 'approve' ? t('Approve') : t('Reject')}
        description={
          target
            ? t('Request #{{id}} from {{username}}', {
                id: target.task.id,
                username: target.task.username,
              })
            : undefined
        }
        footer={
          <div className='flex justify-end gap-2'>
            <Button
              variant='outline'
              onClick={() => setTarget(null)}
              disabled={decideMutation.isPending}
            >
              {t('Cancel')}
            </Button>
            <Button
              variant={target?.action === 'reject' ? 'destructive' : 'default'}
              disabled={confirmDisabled}
              onClick={() => {
                if (!target) return
                decideMutation.mutate({
                  id: target.task.id,
                  action: target.action,
                  comment: comment.trim(),
                })
              }}
            >
              {decideMutation.isPending ? t('Submitting...') : t('Confirm')}
            </Button>
          </div>
        }
      >
        <div className='space-y-2'>
          <Label htmlFor='recharge-decide-comment'>
            {target?.action === 'reject'
              ? t('Reject reason (required)')
              : t('Comment (optional)')}
          </Label>
          <Textarea
            id='recharge-decide-comment'
            rows={3}
            maxLength={500}
            value={comment}
            onChange={(e) => setComment(e.target.value)}
            placeholder={
              target?.action === 'reject'
                ? t('Please explain why this request is rejected...')
                : t('Add a comment for the applicant...')
            }
          />
        </div>
      </Dialog>
    </div>
  )
}
