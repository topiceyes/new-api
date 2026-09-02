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
import { Link, useLocation } from '@tanstack/react-router'
import { ChevronRight } from 'lucide-react'
import { type ReactNode, useState, useEffect } from 'react'

import { Badge } from '@/components/ui/badge'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  SidebarGroup,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
  useSidebar,
} from '@/components/ui/sidebar'

import { checkIsActive } from '../lib/url-utils'
import type {
  NavCollapsible,
  NavChatPresets,
  NavItem,
  NavLink,
  NavGroup as NavGroupProps,
} from '../types'
import { ChatPresetsItem } from './chat-presets-item'

/**
 * Sidebar navigation group component
 * Renders a group of navigation items, supporting regular links and collapsible submenus
 */
export function NavGroup({ id, title, items, collapsible }: NavGroupProps) {
  const { state, isMobile } = useSidebar()
  const href = useLocation({ select: (location) => location.href })

  const menu = (
    <SidebarMenu>
      {items.map((item) => {
        const key = `${item.title}-${item.url || item.type}`

        // Special handling: dynamic chat presets list
        if (item.type === 'chat-presets') {
          return <ChatPresetsItem key={key} item={item as NavChatPresets} />
        }

        // If no sub-items, render regular link
        if (!item.items) {
          return <SidebarMenuLink key={key} item={item as NavLink} href={href} />
        }

        // In collapsed state on non-mobile, render dropdown menu
        if (state === 'collapsed' && !isMobile) {
          return (
            <SidebarMenuCollapsedDropdown
              key={key}
              item={item as NavCollapsible}
              href={href}
            />
          )
        }

        // Render collapsible menu
        return (
          <SidebarMenuCollapsible
            key={key}
            item={item as NavCollapsible}
            href={href}
          />
        )
      })}
    </SidebarMenu>
  )

  const labelClassName =
    'text-muted-foreground/70 px-2 text-[11px] font-medium tracking-wider uppercase'

  // Group-level collapse is opt-in; the desktop icon-collapsed sidebar keeps
  // the classic rendering so items stay reachable via tooltips/dropdowns.
  if (!collapsible || (state === 'collapsed' && !isMobile)) {
    return (
      <SidebarGroup className='px-2 py-1'>
        <SidebarGroupLabel className={labelClassName}>
          {title}
        </SidebarGroupLabel>
        {menu}
      </SidebarGroup>
    )
  }

  return (
    <CollapsibleNavGroup
      storageId={id ?? title}
      title={title}
      items={items}
      href={href}
      labelClassName={labelClassName}
    >
      {menu}
    </CollapsibleNavGroup>
  )
}

/**
 * Group-level collapsible wrapper: the group label acts as the trigger and
 * the whole item list folds away. State persists in localStorage; a group
 * containing the active item always auto-opens (without overwriting the
 * persisted choice).
 */
function CollapsibleNavGroup({
  storageId,
  title,
  items,
  href,
  labelClassName,
  children,
}: {
  storageId: string
  title: string
  items: NavItem[]
  href: string
  labelClassName: string
  children: ReactNode
}) {
  const storageKey = `sidebar-group-open:${storageId}`
  const [isOpen, setIsOpen] = useState(() => {
    try {
      const raw = localStorage.getItem(storageKey)
      return raw === null ? true : raw === 'true'
    } catch {
      return true
    }
  })

  const handleOpenChange = (open: boolean) => {
    setIsOpen(open)
    try {
      localStorage.setItem(storageKey, String(open))
    } catch {
      // Storage can be unavailable in private mode; the toggle still works.
    }
  }

  const hasActiveItem = items.some((item) => checkIsActive(href, item))
  useEffect(() => {
    if (hasActiveItem) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setIsOpen(true)
    }
  }, [hasActiveItem])

  return (
    <Collapsible
      open={isOpen}
      onOpenChange={handleOpenChange}
      render={<SidebarGroup className='px-2 py-1' />}
    >
      <CollapsibleTrigger
        className='group/group-trigger cursor-pointer'
        render={<SidebarGroupLabel className={labelClassName} />}
      >
        <span className='min-w-0 flex-1 truncate'>{title}</span>
        <ChevronRight className='ms-auto size-3.5 shrink-0 transition-transform duration-200 group-data-[panel-open]/group-trigger:rotate-90' />
      </CollapsibleTrigger>
      <CollapsibleContent className='CollapsibleContent'>
        {children}
      </CollapsibleContent>
    </Collapsible>
  )
}

/**
 * Navigation badge component
 */
function NavBadge({ children }: { children: ReactNode }) {
  return <Badge className='shrink-0 px-1 py-0 text-xs'>{children}</Badge>
}

/**
 * Sidebar menu link item
 */
function SidebarMenuLink({ item, href }: { item: NavLink; href: string }) {
  const { isMobile, setOpenMobile } = useSidebar()
  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        isActive={checkIsActive(href, item)}
        tooltip={item.title}
        render={
          <Link
            to={item.url}
            preload={isMobile ? false : undefined}
            onClick={() => setOpenMobile(false)}
          />
        }
      >
        {item.icon && <item.icon className='shrink-0' />}
        <span className='min-w-0 flex-1 truncate'>{item.title}</span>
        {item.badge && <NavBadge>{item.badge}</NavBadge>}
      </SidebarMenuButton>
    </SidebarMenuItem>
  )
}

/**
 * Sidebar collapsible menu item
 */
function SidebarMenuCollapsible({
  item,
  href,
}: {
  item: NavCollapsible
  href: string
}) {
  const { isMobile, setOpenMobile } = useSidebar()
  // 检查当前路径是否匹配子菜单项
  const isSubItemActive = checkIsActive(href, item)
  // 使用受控状态，初始值基于当前路径是否匹配
  const [isOpen, setIsOpen] = useState(() => isSubItemActive)

  // 当路径变化时，如果匹配子菜单项，自动展开父级菜单
  useEffect(() => {
    if (isSubItemActive) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setIsOpen(true)
    }
  }, [isSubItemActive])

  return (
    <Collapsible
      open={isOpen}
      onOpenChange={setIsOpen}
      className='group/collapsible'
      render={<SidebarMenuItem />}
    >
      <CollapsibleTrigger
        className='group/collapsible-trigger'
        render={<SidebarMenuButton tooltip={item.title} />}
      >
        {item.icon && <item.icon className='shrink-0' />}
        <span className='min-w-0 flex-1 truncate'>{item.title}</span>
        {item.badge && <NavBadge>{item.badge}</NavBadge>}
        <ChevronRight className='ms-auto size-4 shrink-0 transition-transform duration-200 group-data-[panel-open]/collapsible-trigger:rotate-90' />
      </CollapsibleTrigger>
      <CollapsibleContent className='CollapsibleContent'>
        <SidebarMenuSub>
          {item.items.map((subItem) => (
            <SidebarMenuSubItem key={subItem.title}>
              <SidebarMenuSubButton
                isActive={checkIsActive(href, subItem)}
                render={
                  <Link
                    to={subItem.url}
                    preload={isMobile ? false : undefined}
                    onClick={() => setOpenMobile(false)}
                  />
                }
              >
                {subItem.icon && <subItem.icon className='shrink-0' />}
                <span className='min-w-0 flex-1 truncate'>{subItem.title}</span>
                {subItem.badge && <NavBadge>{subItem.badge}</NavBadge>}
              </SidebarMenuSubButton>
            </SidebarMenuSubItem>
          ))}
        </SidebarMenuSub>
      </CollapsibleContent>
    </Collapsible>
  )
}

/**
 * Sidebar dropdown menu item when collapsed
 */
function SidebarMenuCollapsedDropdown({
  item,
  href,
}: {
  item: NavCollapsible
  href: string
}) {
  return (
    <SidebarMenuItem>
      <DropdownMenu>
        <DropdownMenuTrigger
          className='group/dropdown-trigger'
          render={
            <SidebarMenuButton
              tooltip={item.title}
              isActive={checkIsActive(href, item)}
            />
          }
        >
          {item.icon && <item.icon className='shrink-0' />}
          <span className='min-w-0 flex-1 truncate'>{item.title}</span>
          {item.badge && <NavBadge>{item.badge}</NavBadge>}
          <ChevronRight className='ms-auto size-4 shrink-0 transition-transform duration-200 group-data-[popup-open]/dropdown-trigger:rotate-90' />
        </DropdownMenuTrigger>
        <DropdownMenuContent side='right' align='start' sideOffset={4}>
          <DropdownMenuGroup>
            <DropdownMenuLabel>
              {item.title} {item.badge ? `(${item.badge})` : ''}
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            {item.items.map((sub) => (
              <DropdownMenuItem
                key={`${sub.title}-${sub.url}`}
                render={
                  <Link
                    to={sub.url}
                    className={`${checkIsActive(href, sub) ? 'bg-secondary' : ''}`}
                  />
                }
              >
                {sub.icon && <sub.icon />}
                <span className='max-w-52 text-wrap'>{sub.title}</span>
                {sub.badge && (
                  <span className='ms-auto text-xs'>{sub.badge}</span>
                )}
              </DropdownMenuItem>
            ))}
          </DropdownMenuGroup>
        </DropdownMenuContent>
      </DropdownMenu>
    </SidebarMenuItem>
  )
}
