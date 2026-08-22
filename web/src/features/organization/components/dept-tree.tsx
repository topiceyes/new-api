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
import { ChevronRight } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { ScrollArea } from '@/components/ui/scroll-area'
import { cn } from '@/lib/utils'

import type { OrgDepartment } from '../types'

interface DeptTreeProps {
  departments: OrgDepartment[]
  selectedDeptId: string
  onSelect: (deptId: string) => void
}

interface DeptNode {
  dept: OrgDepartment
  depth: number
  hasChildren: boolean
  expanded: boolean
}

// 部门树:按 parent_id 组树,可折叠。默认只展开根部门(parent_id 为空),
// 其余有子部门的节点折叠,避免全量展开时列表过长。用户手动折叠/展开的
// 状态保存在 collapsedIds 里;为 null 表示未碰过,跟随默认折叠集合,
// 这样快照刷新后未操作的树仍能按新数据取默认。
export function DeptTree({
  departments,
  selectedDeptId,
  onSelect,
}: DeptTreeProps) {
  const { t } = useTranslation()
  const [collapsedIds, setCollapsedIds] = useState<Set<string> | null>(null)

  const childrenByParent = useMemo(() => {
    const map = new Map<string, OrgDepartment[]>()
    for (const dept of departments) {
      const siblings = map.get(dept.parent_id) ?? []
      siblings.push(dept)
      map.set(dept.parent_id, siblings)
    }
    return map
  }, [departments])

  const defaultCollapsed = useMemo(() => {
    const set = new Set<string>()
    for (const dept of departments) {
      if (dept.parent_id !== '' && childrenByParent.has(dept.dept_id)) {
        set.add(dept.dept_id)
      }
    }
    return set
  }, [departments, childrenByParent])

  const effectiveCollapsed = collapsedIds ?? defaultCollapsed

  const nodes = useMemo<DeptNode[]>(() => {
    const flat: DeptNode[] = []
    const walk = (parentId: string, depth: number) => {
      for (const dept of childrenByParent.get(parentId) ?? []) {
        const hasChildren = childrenByParent.has(dept.dept_id)
        const expanded = hasChildren && !effectiveCollapsed.has(dept.dept_id)
        flat.push({ dept, depth, hasChildren, expanded })
        if (expanded) walk(dept.dept_id, depth + 1)
      }
    }
    walk('', 0)
    return flat
  }, [childrenByParent, effectiveCollapsed])

  const toggle = (deptId: string) => {
    const next = new Set(effectiveCollapsed)
    if (next.has(deptId)) {
      next.delete(deptId)
    } else {
      next.add(deptId)
    }
    setCollapsedIds(next)
  }

  return (
    <ScrollArea className='h-full'>
      <div className='flex flex-col gap-0.5 p-2'>
        {nodes.map(({ dept, depth, hasChildren, expanded }) => (
          <div
            key={dept.dept_id}
            className={cn(
              'flex items-center rounded-md pr-1 text-sm transition-colors',
              selectedDeptId === dept.dept_id
                ? 'bg-primary text-primary-foreground'
                : 'hover:bg-muted'
            )}
            style={{ marginLeft: `${depth * 16}px` }}
          >
            {hasChildren ? (
              <button
                type='button'
                aria-label={expanded ? t('Collapse') : t('Expand')}
                onClick={() => toggle(dept.dept_id)}
                className='shrink-0 rounded p-1 opacity-70 hover:opacity-100'
              >
                <ChevronRight
                  className={cn(
                    'h-3.5 w-3.5 transition-transform',
                    expanded && 'rotate-90'
                  )}
                />
              </button>
            ) : (
              <span className='w-[26px] shrink-0' />
            )}
            <button
              type='button'
              onClick={() => onSelect(dept.dept_id)}
              className='flex min-w-0 flex-1 items-center gap-1 py-1.5 pr-1 text-left'
            >
              <span className='truncate'>{dept.name}</span>
              <Badge
                variant={
                  selectedDeptId === dept.dept_id ? 'outline' : 'secondary'
                }
                className='ml-auto shrink-0'
              >
                {dept.member_count}
              </Badge>
            </button>
          </div>
        ))}
      </div>
    </ScrollArea>
  )
}
