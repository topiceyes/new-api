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
import type { RechargeCategory, RechargeLevelType } from './types'

// 枚举展示文案:i18n 键(与英文源串同字面量),组件中通过 t() 渲染。
export const CATEGORY_LABEL_KEY: Record<RechargeCategory, string> = {
  project_delivery: 'Project Delivery',
  tech_research: 'Tech Research',
}

export const LEVEL_TYPE_LABEL_KEY: Record<RechargeLevelType, string> = {
  dept_leader: 'Department Leader',
  designated: 'Designated Approver',
}
