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
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import { parseIdList, type OrgDepartment, type OrgMember } from '../types'

interface MemberTableProps {
  members: OrgMember[]
  departments: OrgDepartment[]
  /** 空串表示不按部门过滤(未选部门时一般不会出现,防御用) */
  selectedDeptId: string
}

// 成员表:姓名/职位/部门/主管/绑定状态。主管徽标按「在选中部门内是主管」
// 判定,避免一人主管 A 部门却在 B 部门成员列表里误挂徽标。
export function MemberTable({
  members,
  departments,
  selectedDeptId,
}: MemberTableProps) {
  const { t } = useTranslation()

  const deptNameById = useMemo(() => {
    const map = new Map<string, string>()
    for (const dept of departments) {
      map.set(dept.dept_id, dept.name)
    }
    return map
  }, [departments])

  const filtered = useMemo(() => {
    if (!selectedDeptId) return members
    return members.filter((m) =>
      parseIdList(m.dept_ids).includes(selectedDeptId)
    )
  }, [members, selectedDeptId])

  if (filtered.length === 0) {
    return (
      <div className='text-muted-foreground py-12 text-center'>
        {t('No members in this department.')}
      </div>
    )
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t('Name')}</TableHead>
          <TableHead>{t('Position')}</TableHead>
          <TableHead>{t('Department')}</TableHead>
          <TableHead>{t('Supervisor')}</TableHead>
          <TableHead>{t('Account')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {filtered.map((member) => {
          const deptNames = parseIdList(member.dept_ids)
            .map((id) => deptNameById.get(id) ?? id)
            .join(' / ')
          const isLeaderHere = parseIdList(member.leader_dept_ids).includes(
            selectedDeptId
          )
          return (
            <TableRow key={member.union_id}>
              <TableCell className='font-medium'>{member.name}</TableCell>
              <TableCell className='text-muted-foreground'>
                {member.title || '-'}
              </TableCell>
              <TableCell className='text-muted-foreground'>
                {deptNames}
              </TableCell>
              <TableCell>
                {isLeaderHere && <Badge>{t('Supervisor')}</Badge>}
              </TableCell>
              <TableCell>
                {member.user_id > 0 ? (
                  <span>
                    {member.display_name || member.username}
                    {member.display_name && member.username && (
                      <span className='text-muted-foreground'>
                        {' '}
                        ({member.username})
                      </span>
                    )}
                  </span>
                ) : (
                  <Badge variant='secondary'>{t('Not bound')}</Badge>
                )}
              </TableCell>
            </TableRow>
          )
        })}
      </TableBody>
    </Table>
  )
}
