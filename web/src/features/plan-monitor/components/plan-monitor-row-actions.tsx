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
import { useQueryClient } from '@tanstack/react-query'
import {
  Loader2,
  Pencil,
  Power,
  PowerOff,
  RefreshCw,
  Trash2,
} from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'

import { deletePlan, refreshPlan, setPlanStatus } from '../api'
import type { PlanMonitorPlan } from '../types'

interface Props {
  plan: PlanMonitorPlan
  onEdit: (plan: PlanMonitorPlan) => void
}

export function PlanMonitorRowActions({ plan, onEdit }: Props) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isEnabled = plan.enabled
  const toggleLabel = isEnabled ? t('Disable') : t('Enable')

  const [refreshing, setRefreshing] = useState(false)
  const [toggling, setToggling] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleting, setDeleting] = useState(false)

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ['plan-monitor-plans'] })
    queryClient.invalidateQueries({ queryKey: ['plan-monitor-overview'] })
  }

  const handleToggleStatus = async () => {
    setToggling(true)
    try {
      const res = await setPlanStatus(plan.id, !isEnabled)
      if (res.success) {
        toast.success(
          isEnabled ? t('Has been disabled') : t('Has been enabled')
        )
        invalidate()
      } else {
        toast.error(res.message || t('Operation failed'))
      }
    } catch {
      toast.error(t('Operation failed'))
    } finally {
      setToggling(false)
    }
  }

  const handleRefresh = async () => {
    setRefreshing(true)
    try {
      const res = await refreshPlan(plan.id)
      if (res.success) {
        toast.success(t('Refresh triggered'))
        invalidate()
      } else {
        toast.error(res.message || t('Refresh failed'))
      }
    } catch {
      toast.error(t('Refresh failed'))
    } finally {
      setRefreshing(false)
    }
  }

  const handleDelete = async () => {
    setDeleting(true)
    try {
      const res = await deletePlan(plan.id)
      if (res.success) {
        toast.success(t('Delete succeeded'))
        setDeleteOpen(false)
        invalidate()
      } else {
        toast.error(res.message || t('Delete failed'))
      }
    } catch {
      toast.error(t('Delete failed'))
    } finally {
      setDeleting(false)
    }
  }

  const renderToggleIcon = () => {
    if (toggling) return <Loader2 className='animate-spin' />
    return isEnabled ? <PowerOff /> : <Power />
  }

  return (
    <div className='-ml-1.5 flex items-center gap-1'>
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant='ghost'
              size='icon-sm'
              onClick={() => onEdit(plan)}
              aria-label={t('Edit')}
            />
          }
        >
          <Pencil />
        </TooltipTrigger>
        <TooltipContent>{t('Edit')}</TooltipContent>
      </Tooltip>

      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant='ghost'
              size='icon-sm'
              onClick={handleToggleStatus}
              disabled={toggling}
              aria-label={toggleLabel}
              className={
                isEnabled
                  ? 'text-destructive hover:text-destructive'
                  : 'text-success hover:text-success'
              }
            />
          }
        >
          {renderToggleIcon()}
        </TooltipTrigger>
        <TooltipContent>{toggleLabel}</TooltipContent>
      </Tooltip>

      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant='ghost'
              size='icon-sm'
              onClick={handleRefresh}
              disabled={refreshing}
              aria-label={t('Refresh now')}
            />
          }
        >
          {refreshing ? <Loader2 className='animate-spin' /> : <RefreshCw />}
        </TooltipTrigger>
        <TooltipContent>{t('Refresh now')}</TooltipContent>
      </Tooltip>

      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant='ghost'
              size='icon-sm'
              onClick={() => setDeleteOpen(true)}
              aria-label={t('Delete')}
              className='text-destructive hover:text-destructive'
            />
          }
        >
          <Trash2 />
        </TooltipTrigger>
        <TooltipContent>{t('Delete')}</TooltipContent>
      </Tooltip>

      <AlertDialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Confirm delete')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'Are you sure you want to delete the plan monitor "{{name}}"? This action cannot be undone.',
                { name: plan.plan_name }
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleting}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDelete}
              disabled={deleting}
              variant='destructive'
            >
              {deleting ? t('Deleting...') : t('Delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
