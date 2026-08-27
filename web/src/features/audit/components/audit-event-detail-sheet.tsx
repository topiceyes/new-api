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
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import {
  SideDrawerSection,
  SideDrawerSectionHeader,
  sideDrawerContentClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'

import { getAuditEvent } from '../api'
import type { AuditEvent } from '../types'

interface Props {
  event: AuditEvent | undefined
  open: boolean
  onOpenChange: (open: boolean) => void
}

function DetailRow({ label, value }: { label: string; value?: string | number }) {
  return (
    <div className='grid grid-cols-[110px_1fr] gap-2 py-1 text-sm'>
      <span className='text-muted-foreground'>{label}</span>
      <span className='break-all'>{value === '' || value == null ? '-' : value}</span>
    </div>
  )
}

export function AuditEventDetailSheet({ event, open, onOpenChange }: Props) {
  const { t } = useTranslation()

  // 列表行不含 prompt 原文(列表接口已剔除),打开详情时单独拉取。
  const { data: detail } = useQuery({
    queryKey: ['audit-event', event?.id],
    queryFn: async () => {
      const result = await getAuditEvent(event!.id)
      return result.success ? result.data : event!
    },
    enabled: open && event != null,
    initialData: event,
  })

  let detailObj: Record<string, unknown> | null = null
  try {
    if (detail?.detail) {
      detailObj = JSON.parse(detail.detail) as Record<string, unknown>
    }
  } catch {
    detailObj = null
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className={sideDrawerContentClassName('sm:max-w-[640px]')}>
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>{t('Audit Event Detail')}</SheetTitle>
        </SheetHeader>
        {detail && (
          <div className='space-y-4 overflow-y-auto px-4 pb-4'>
            <SideDrawerSection>
              <SideDrawerSectionHeader title={t('Basic Info')} />
              <DetailRow label={t('ID')} value={detail.id} />
              <DetailRow
                label={t('Time')}
                value={new Date(detail.created_at * 1000).toLocaleString()}
              />
              <DetailRow label={t('Event Type')} value={detail.event_type} />
              <DetailRow label={t('Severity')} value={t(detail.severity)} />
              <DetailRow label={t('Request ID')} value={detail.request_id} />
            </SideDrawerSection>

            <SideDrawerSection>
              <SideDrawerSectionHeader title={t('Caller')} />
              <DetailRow
                label={t('Username')}
                value={`${detail.username} (ID: ${detail.user_id})`}
              />
              <DetailRow
                label={t('Token')}
                value={`${detail.token_name || '-'} (ID: ${detail.token_id})`}
              />
              <DetailRow label={t('Group')} value={detail.group} />
              <DetailRow label={t('Model')} value={detail.model_name} />
              {detail.category && (
                <DetailRow label={t('Category')} value={detail.category} />
              )}
              <DetailRow label={t('IP')} value={detail.ip} />
              <DetailRow label={t('User-Agent')} value={detail.user_agent} />
            </SideDrawerSection>

            {(detail.rule_id || detail.excerpt) && (
              <SideDrawerSection>
                <SideDrawerSectionHeader title={t('Rule Hit')} />
                <DetailRow label={t('Rule')} value={detail.rule_name} />
                <DetailRow label={t('Rule ID')} value={detail.rule_id} />
                <DetailRow label={t('Excerpt')} value={detail.excerpt} />
                {detailObj && (
                  <DetailRow
                    label={t('Hit Count')}
                    value={String(detailObj.count ?? '-')}
                  />
                )}
              </SideDrawerSection>
            )}

            {detailObj && !detail.rule_id && (
              <SideDrawerSection>
                <SideDrawerSectionHeader title={t('Signal Detail')} />
                {Object.entries(detailObj).map(([key, value]) => (
                  <DetailRow key={key} label={key} value={String(value)} />
                ))}
              </SideDrawerSection>
            )}

            <SideDrawerSection>
              <SideDrawerSectionHeader title={t('Prompt Content')} />
              {detail.prompt ? (
                <pre className='bg-muted max-h-[320px] overflow-auto rounded-md p-3 text-xs break-all whitespace-pre-wrap'>
                  {detail.prompt}
                </pre>
              ) : (
                <p className='text-muted-foreground text-sm'>
                  {t(
                    'Prompt content was not stored. Enable prompt storage in Security Settings to retain it.'
                  )}
                </p>
              )}
            </SideDrawerSection>
          </div>
        )}
      </SheetContent>
    </Sheet>
  )
}
