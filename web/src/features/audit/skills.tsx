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

import { AuditNavTabs } from './components/audit-nav-tabs'
import { SkillCandidatesTable } from './components/skill-candidates-table'
import { SkillsTable } from './components/skills-table'

export function AuditSkillsPage() {
  const { t } = useTranslation()
  const [tab, setTab] = useState<'candidates' | 'library'>('candidates')

  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>{t('Security Audit')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='mb-4 flex flex-wrap items-center gap-4'>
          <AuditNavTabs active='skills' />
          <div className='flex gap-2'>
            <button
              type='button'
              onClick={() => setTab('candidates')}
              className={
                tab === 'candidates'
                  ? 'text-foreground border-primary border-b-2 px-1 pb-1 text-sm font-medium'
                  : 'text-muted-foreground px-1 pb-1 text-sm'
              }
            >
              {t('Pending Candidates')}
            </button>
            <button
              type='button'
              onClick={() => setTab('library')}
              className={
                tab === 'library'
                  ? 'text-foreground border-primary border-b-2 px-1 pb-1 text-sm font-medium'
                  : 'text-muted-foreground px-1 pb-1 text-sm'
              }
            >
              {t('Published Skills')}
            </button>
          </div>
        </div>
        {tab === 'candidates' ? <SkillCandidatesTable /> : <SkillsTable />}
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
