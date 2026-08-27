/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR a PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

/** 安全审计下的子页导航:事件列表 / skill 库。Tabs 组件不支持 asChild,
 * 用 onClick 导航代替 Link 包裹。 */
export function AuditNavTabs({ active }: { active: 'events' | 'skills' }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  return (
    <Tabs value={active}>
      <TabsList>
        <TabsTrigger
          value='events'
          onClick={() => navigate({ to: '/audit' })}
        >
          {t('Audit Events')}
        </TabsTrigger>
        <TabsTrigger
          value='skills'
          onClick={() => navigate({ to: '/audit/skills' })}
        >
          {t('Skill Library')}
        </TabsTrigger>
      </TabsList>
    </Tabs>
  )
}
