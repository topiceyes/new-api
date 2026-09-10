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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { AdminRequestsTable } from './components/admin-requests-table'
import { ApplyForm } from './components/apply-form'
import { MyApprovalsTable } from './components/my-approvals-table'
import { MyRequestsTable } from './components/my-requests-table'

type RechargeTab = 'apply' | 'mine' | 'approvals' | 'all'

// 充值申请 + 多级审批用户页:发起申请 / 我的申请 / 我的审批,管理员额外可见全部申请。
export function RechargeRequestsPage() {
  const { t } = useTranslation()
  const user = useAuthStore((s) => s.auth.user)
  const isAdmin = (user?.role ?? ROLE.GUEST) >= ROLE.ADMIN
  const [tab, setTab] = useState<RechargeTab>('apply')

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Recharge Requests')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='space-y-4'>
          <Tabs
            value={tab}
            onValueChange={(value) => setTab(value as RechargeTab)}
          >
            <TabsList className='max-w-full flex-wrap justify-start group-data-horizontal/tabs:h-auto'>
              <TabsTrigger value='apply'>{t('New Request')}</TabsTrigger>
              <TabsTrigger value='mine'>{t('My Requests')}</TabsTrigger>
              <TabsTrigger value='approvals'>{t('My Approvals')}</TabsTrigger>
              {isAdmin && (
                <TabsTrigger value='all'>{t('All Requests')}</TabsTrigger>
              )}
            </TabsList>
          </Tabs>

          {tab === 'apply' && <ApplyForm />}
          {tab === 'mine' && <MyRequestsTable />}
          {tab === 'approvals' && <MyApprovalsTable />}
          {tab === 'all' && isAdmin && <AdminRequestsTable />}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
