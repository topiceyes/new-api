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
import { useQuery } from '@tanstack/react-query'
import type { UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { getGroups } from '@/features/subscriptions/api'

import {
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { safeNumberFieldProps } from '../utils/numeric-field'
import type { OAuthFormValues } from './oauth-section'

export type OrgSyncProvider = 'dingtalk' | 'feishu'

type OrgSyncBlockProps = {
  form: UseFormReturn<OAuthFormValues>
  provider: OrgSyncProvider
}

// 组织架构同步配置块(钉钉/飞书两个 tab 复用):启用开关 + 同步间隔 +
// 可选的「主管映射到分组」。同步数据源跟随登录 provider,离职处理归巡检。
export function OrgSyncBlock(props: OrgSyncBlockProps) {
  const { t } = useTranslation()
  const provider = props.provider
  const providerName = provider === 'feishu' ? t('Feishu') : t('DingTalk')

  const enabled = props.form.watch(`${provider}.orgsync_enabled`)
  const mapGroup = props.form.watch(`${provider}.orgsync_map_group`)

  const { data: groupsData } = useQuery({
    queryKey: ['org-sync-groups'],
    queryFn: async () => {
      const res = await getGroups()
      return res.success ? (res.data ?? []) : []
    },
    // 分组列表只在开启映射时才需要;开关默认关,避免无谓请求。
    enabled: mapGroup === true,
  })
  const groups = groupsData ?? []

  return (
    <>
      <div className='border-t pt-5 text-sm font-medium lg:col-span-2'>
        {t('Organization sync')}
      </div>

      <FormField
        control={props.form.control}
        name={`${provider}.orgsync_enabled`}
        render={({ field }) => (
          <SettingsSwitchItem>
            <SettingsSwitchContent>
              <FormLabel>{t('Enable organization sync')}</FormLabel>
              <FormDescription>
                {t(
                  'Periodically sync departments and members from the {{provider}} address book; view the snapshot under Admin - Organization',
                  { provider: providerName }
                )}
              </FormDescription>
            </SettingsSwitchContent>
            <FormControl>
              <Switch checked={field.value} onCheckedChange={field.onChange} />
            </FormControl>
          </SettingsSwitchItem>
        )}
      />

      {enabled ? (
        <>
          <FormField
            control={props.form.control}
            name={`${provider}.orgsync_interval_hours`}
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Sync interval (hours, 1-168)')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    min={1}
                    max={168}
                    step={1}
                    {...safeNumberFieldProps(field)}
                  />
                </FormControl>
                <FormDescription>
                  {t('How often the organization snapshot is refreshed')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={props.form.control}
            name={`${provider}.orgsync_map_group`}
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Map supervisors to a user group')}</FormLabel>
                  <FormDescription>
                    {t(
                      'During sync, supervisors (department leaders) bound to local accounts are moved into the target group, and restored when they are no longer supervisors'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          {mapGroup ? (
            <FormField
              control={props.form.control}
              name={`${provider}.orgsync_target_group`}
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Supervisor target group')}</FormLabel>
                  <Select
                    items={groups.map((g) => ({ value: g, label: g }))}
                    value={field.value}
                    onValueChange={field.onChange}
                  >
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue placeholder={t('Select a user group')} />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent alignItemWithTrigger={false}>
                      <SelectGroup>
                        {groups.map((g) => (
                          <SelectItem key={g} value={g}>
                            {g}
                          </SelectItem>
                        ))}
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                  <FormDescription>
                    {t(
                      'Only groups already defined in user group settings are accepted'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          ) : null}
        </>
      ) : null}
    </>
  )
}
