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
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { formatQuotaWithCurrency } from '@/lib/currency'
import { formatTimestampToDate } from '@/lib/format'

import { getRechargeRequestDetail } from '../api'
import { CATEGORY_LABEL_KEY, LEVEL_TYPE_LABEL_KEY } from '../constants'
import type { ApprovalStep } from '../types'
import { RechargeDecisionBadge, RechargeStatusBadge } from './badges'

const SKELETON_KEYS = ['sk-1', 'sk-2', 'sk-3', 'sk-4']

interface RequestDetailDialogProps {
  requestId: number | null
  open: boolean
  onOpenChange: (open: boolean) => void
  admin?: boolean
}

function groupStepsByLevel(steps: ApprovalStep[]): Map<number, ApprovalStep[]> {
  const grouped = new Map<number, ApprovalStep[]>()
  for (const step of steps) {
    const list = grouped.get(step.level) ?? []
    list.push(step)
    grouped.set(step.level, list)
  }
  return grouped
}

export function RequestDetailDialog(props: RequestDetailDialogProps) {
  const { t } = useTranslation()
  const admin = props.admin ?? false

  const detailQuery = useQuery({
    queryKey: ['recharge-request-detail', admin, props.requestId],
    queryFn: async () => {
      if (props.requestId === null) throw new Error('missing request id')
      const res = await getRechargeRequestDetail(props.requestId, admin)
      if (!res.success) throw new Error(res.message)
      return res.data
    },
    enabled: props.open && props.requestId !== null,
  })

  const request = detailQuery.data?.request
  const steps = detailQuery.data?.steps ?? []
  const groupedSteps = groupStepsByLevel(steps)
  const levels = [...groupedSteps.entries()].sort((a, b) => a[0] - b[0])

  let body: ReactNode
  if (detailQuery.isLoading || !request) {
    body = (
      <div className='space-y-3'>
        {SKELETON_KEYS.map((key) => (
          <Skeleton key={key} className='h-12 w-full rounded-md' />
        ))}
      </div>
    )
  } else if (detailQuery.isError) {
    body = (
      <div className='text-muted-foreground py-8 text-center text-sm'>
        {t('Failed to load request detail.')}
      </div>
    )
  } else {
    body = (
      <>
        <div className='grid grid-cols-2 gap-3 sm:grid-cols-3'>
          <div className='space-y-1'>
            <Label className='text-muted-foreground text-xs'>
              {t('Applicant')}
            </Label>
            <div className='text-sm font-medium'>{request.username}</div>
          </div>
          <div className='space-y-1'>
            <Label className='text-muted-foreground text-xs'>
              {t('Category')}
            </Label>
            <div className='text-sm font-medium'>
              {t(CATEGORY_LABEL_KEY[request.category])}
            </div>
          </div>
          <div className='space-y-1'>
            <Label className='text-muted-foreground text-xs'>
              {t('Amount')}
            </Label>
            <div className='text-sm font-medium'>
              {formatQuotaWithCurrency(request.quota)}
            </div>
          </div>
          <div className='space-y-1'>
            <Label className='text-muted-foreground text-xs'>
              {t('Status')}
            </Label>
            <div>
              <RechargeStatusBadge status={request.status} />
            </div>
          </div>
          <div className='space-y-1'>
            <Label className='text-muted-foreground text-xs'>
              {t('Created At')}
            </Label>
            <div className='text-sm'>
              {formatTimestampToDate(request.create_time)}
            </div>
          </div>
          <div className='space-y-1'>
            <Label className='text-muted-foreground text-xs'>
              {t('Completed At')}
            </Label>
            <div className='text-sm'>
              {formatTimestampToDate(request.complete_time)}
            </div>
          </div>
        </div>

        <div className='space-y-1'>
          <Label className='text-muted-foreground text-xs'>
            {t('Details')}
          </Label>
          <div className='rounded-md border p-3 text-sm break-words whitespace-pre-wrap'>
            {request.detail}
          </div>
        </div>

        {request.status === 'rejected' && request.reject_reason && (
          <div className='space-y-1'>
            <Label className='text-muted-foreground text-xs'>
              {t('Reject Reason')}
            </Label>
            <div className='rounded-md border border-red-200 p-3 text-sm text-red-700 dark:border-red-500/30 dark:text-red-300'>
              {request.reject_reason}
            </div>
          </div>
        )}

        <div className='space-y-2'>
          <Label className='text-muted-foreground text-xs'>
            {t('Approval Progress')}
          </Label>
          <ol className='relative space-y-4 border-l pl-4'>
            {levels.map(([level, levelSteps]) => (
              <li key={level} className='relative'>
                <span
                  className='bg-muted-foreground/40 absolute top-1.5 -left-[21px] size-2 rounded-full'
                  aria-hidden='true'
                />
                <div className='mb-1 text-sm font-medium'>
                  {t('Level {{level}}', { level })}
                </div>
                <ul className='space-y-2'>
                  {levelSteps.map((step) => (
                    <li
                      key={step.id}
                      className='flex flex-col gap-1 rounded-md border p-2.5'
                    >
                      <div className='flex items-center justify-between gap-2'>
                        <span className='text-sm'>
                          {step.name || `#${step.user_id}`}
                          <span className='text-muted-foreground ml-2 text-xs'>
                            {t(LEVEL_TYPE_LABEL_KEY[step.level_type])}
                          </span>
                        </span>
                        <RechargeDecisionBadge decision={step.decision} />
                      </div>
                      {step.comment && (
                        <div className='text-muted-foreground text-xs break-words'>
                          {step.comment}
                        </div>
                      )}
                      {step.decide_time > 0 && (
                        <div className='text-muted-foreground text-xs'>
                          {formatTimestampToDate(step.decide_time)}
                        </div>
                      )}
                    </li>
                  ))}
                </ul>
              </li>
            ))}
          </ol>
        </div>
      </>
    )
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Request Detail')}
      description={request ? `#${request.id}` : undefined}
      contentClassName='sm:max-w-2xl'
      bodyClassName='space-y-4'
    >
      {body}
    </Dialog>
  )
}
