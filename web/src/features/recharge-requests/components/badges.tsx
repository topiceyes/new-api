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
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

import type { ApprovalDecision, RechargeRequestStatus } from '../types'

const STATUS_LABEL_KEY: Record<RechargeRequestStatus, string> = {
  pending: 'Pending',
  approved: 'Approved',
  rejected: 'Rejected',
}

const STATUS_CLASS_NAME: Record<RechargeRequestStatus, string> = {
  pending:
    'bg-amber-50 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300',
  approved:
    'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300',
  rejected: 'bg-red-50 text-red-700 dark:bg-red-500/15 dark:text-red-300',
}

const DECISION_LABEL_KEY: Record<ApprovalDecision, string> = {
  pending: 'Pending',
  approved: 'Approved',
  rejected: 'Rejected',
  skipped: 'Skipped',
}

const DECISION_CLASS_NAME: Record<ApprovalDecision, string> = {
  pending:
    'bg-amber-50 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300',
  approved:
    'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300',
  rejected: 'bg-red-50 text-red-700 dark:bg-red-500/15 dark:text-red-300',
  skipped: '',
}

export function RechargeStatusBadge(props: { status: RechargeRequestStatus }) {
  const { t } = useTranslation()
  return (
    <Badge variant='secondary' className={cn(STATUS_CLASS_NAME[props.status])}>
      {t(STATUS_LABEL_KEY[props.status])}
    </Badge>
  )
}

export function RechargeDecisionBadge(props: { decision: ApprovalDecision }) {
  const { t } = useTranslation()
  return (
    <Badge
      variant='secondary'
      className={cn(DECISION_CLASS_NAME[props.decision])}
    >
      {t(DECISION_LABEL_KEY[props.decision])}
    </Badge>
  )
}
