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
*/
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'

import { AuditEventDetailSheet } from './components/audit-event-detail-sheet'
import { AuditEventsTable } from './components/audit-events-table'
import { AuditNavTabs } from './components/audit-nav-tabs'
import type { AuditEvent } from './types'

export function AuditEventsPage() {
  const { t } = useTranslation()
  const [currentEvent, setCurrentEvent] = useState<AuditEvent | undefined>(
    undefined
  )
  const [detailOpen, setDetailOpen] = useState(false)

  const handleView = (event: AuditEvent) => {
    setCurrentEvent(event)
    setDetailOpen(true)
  }

  return (
    <>
      <SectionPageLayout fixedContent>
        <SectionPageLayout.Title>{t('Security Audit')}</SectionPageLayout.Title>
        <SectionPageLayout.Content>
          <div className='mb-4'>
            <AuditNavTabs active='events' />
          </div>
          <AuditEventsTable onView={handleView} />
        </SectionPageLayout.Content>
      </SectionPageLayout>
      <AuditEventDetailSheet
        event={currentEvent}
        open={detailOpen}
        onOpenChange={setDetailOpen}
      />
    </>
  )
}
