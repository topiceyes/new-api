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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { RefreshCw } from 'lucide-react'
import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { formatTimestamp } from '@/lib/format'

import { getOrgOverview, getOrgSyncStatus, runOrgSync } from './api'
import { DeptTree } from './components/dept-tree'
import { MemberTable } from './components/member-table'

// 管理端「组织架构」:展示从钉钉/飞书同步来的部门树与成员快照。
// 手动同步入队 org_sync 系统任务后轮询状态,完成时刷新快照并提示结果。
// 布局用 fixedContent:左右两个面板各自内部滚动,长部门列表不会把
// 成员表顶出视口。
export function Organization() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [selectedDeptId, setSelectedDeptId] = useState('')
  const lastTaskStatus = useRef<string>('')

  const { data, isLoading } = useQuery({
    queryKey: ['org-overview'],
    queryFn: async () => {
      const res = await getOrgOverview()
      if (!res.success) throw new Error(res.message)
      return res.data
    },
  })

  const { data: statusData } = useQuery({
    queryKey: ['org-sync-status'],
    queryFn: async () => {
      const res = await getOrgSyncStatus()
      if (!res.success) throw new Error(res.message)
      return res.data.task
    },
    // 有待执行/执行中的同步任务时 3 秒轮询,静默期不轮询。
    refetchInterval: (query) => {
      const task = query.state.data
      return task && (task.status === 'pending' || task.status === 'running')
        ? 3000
        : false
    },
  })

  const task = statusData ?? null
  const syncing =
    task !== null && (task.status === 'pending' || task.status === 'running')

  // 任务状态收敛到 succeeded/failed 时刷新快照并提示结果。
  useEffect(() => {
    if (!task) return
    const prev = lastTaskStatus.current
    lastTaskStatus.current = task.status
    if (prev !== 'pending' && prev !== 'running') return
    if (task.status === 'succeeded') {
      queryClient.invalidateQueries({ queryKey: ['org-overview'] })
      const r = task.result
      toast.success(
        t(
          'Organization sync finished: {{departments}} departments, {{members}} members, {{matched}} bound.',
          {
            departments: r?.departments ?? 0,
            members: r?.members ?? 0,
            matched: r?.matched ?? 0,
          }
        )
      )
    } else if (task.status === 'failed') {
      toast.error(
        t('Organization sync failed: {{error}}', { error: task.error })
      )
    }
  }, [task, queryClient, t])

  const departments = useMemo(() => data?.departments ?? [], [data])
  const members = useMemo(() => data?.members ?? [], [data])

  // 默认选中根部门(parent_id 为空)。
  useEffect(() => {
    if (!selectedDeptId && departments.length > 0) {
      const root = departments.find((d) => d.parent_id === '')
      setSelectedDeptId((root ?? departments[0]).dept_id)
    }
  }, [departments, selectedDeptId])

  const onSyncNow = async () => {
    const res = await runOrgSync()
    if (!res.success) {
      toast.info(res.message)
    } else {
      toast.success(t('Sync task queued.'))
    }
    lastTaskStatus.current = 'pending'
    queryClient.invalidateQueries({ queryKey: ['org-sync-status'] })
  }

  const providerName = data?.provider === 'feishu' ? t('Feishu') : t('DingTalk')

  let content: ReactNode
  if (isLoading) {
    content = (
      <div className='text-muted-foreground py-12 text-center'>
        {t('Loading...')}
      </div>
    )
  } else if (departments.length === 0) {
    content = (
      <div className='text-muted-foreground py-12 text-center'>
        {t('No organization data yet. Run a sync to fetch the address book.')}
      </div>
    )
  } else {
    content = (
      <div className='flex h-full min-h-0 flex-col gap-3'>
        <div className='text-muted-foreground flex shrink-0 items-center gap-3 text-sm'>
          <Badge variant='outline'>{providerName}</Badge>
          <span>
            {t('Last synced')}:{' '}
            {data?.synced_at
              ? formatTimestamp(data.synced_at)
              : t('Never synced')}
          </span>
          <span>
            {t('Departments')}: {departments.length} · {t('Members')}:{' '}
            {members.length}
          </span>
        </div>
        <div className='grid min-h-0 flex-1 grid-cols-1 grid-rows-[minmax(0,1fr)_minmax(0,1fr)] gap-4 md:grid-cols-[260px_1fr] md:grid-rows-1'>
          <Card className='h-full min-h-0 gap-0 py-0'>
            <CardContent className='h-full min-h-0 p-0'>
              <DeptTree
                departments={departments}
                selectedDeptId={selectedDeptId}
                onSelect={setSelectedDeptId}
              />
            </CardContent>
          </Card>
          <Card className='h-full min-h-0 gap-0 py-0'>
            <CardContent className='h-full min-h-0 overflow-y-auto p-2'>
              <MemberTable
                members={members}
                departments={departments}
                selectedDeptId={selectedDeptId}
              />
            </CardContent>
          </Card>
        </div>
      </div>
    )
  }

  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>{t('Organization')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button
          variant='outline'
          size='sm'
          onClick={onSyncNow}
          disabled={syncing}
        >
          <RefreshCw
            className={`mr-2 h-4 w-4 ${syncing ? 'animate-spin' : ''}`}
          />
          {syncing ? t('Syncing...') : t('Sync Now')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>{content}</SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
