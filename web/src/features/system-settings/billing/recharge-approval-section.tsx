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
import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import { Plus, Search, X } from 'lucide-react'
import { useMemo, useState, type ReactNode } from 'react'
import { useFieldArray, useForm, type Resolver } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Form,
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
import { getUser, searchUsers } from '@/features/users/api'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

const MAX_LEVELS = 5
const MAX_DEPT_LEVEL = 5

const levelSchema = z.object({
  type: z.enum(['dept_leader', 'designated']),
  deptLevel: z.coerce.number().int().min(1).max(MAX_DEPT_LEVEL),
  userIds: z.array(z.number().int().positive()),
})

const schema = z.object({
  enabled: z.boolean(),
  quotaUsd: z.coerce.number().min(0.01),
  levels: z.array(levelSchema).max(MAX_LEVELS),
})

type LevelValue = z.infer<typeof levelSchema>
type Values = z.infer<typeof schema>

// 后端 recharge_approval.levels 选项存的是 JSON 字符串:
// [{type:'dept_leader',dept_level:n} | {type:'designated',user_ids:[...]}]
function parseLevelsJson(json: string): LevelValue[] {
  try {
    const parsed: unknown = JSON.parse(json)
    if (!Array.isArray(parsed)) return []
    const levels: LevelValue[] = []
    for (const raw of parsed) {
      if (typeof raw !== 'object' || raw === null) continue
      const item = raw as Record<string, unknown>
      if (item.type === 'dept_leader') {
        const deptLevel =
          typeof item.dept_level === 'number' ? item.dept_level : 1
        levels.push({ type: 'dept_leader', deptLevel, userIds: [] })
      } else if (item.type === 'designated') {
        const userIds = Array.isArray(item.user_ids)
          ? item.user_ids.filter(
              (id): id is number => typeof id === 'number' && id > 0
            )
          : []
        levels.push({ type: 'designated', deptLevel: 1, userIds })
      }
    }
    return levels
  } catch {
    return []
  }
}

function serializeLevels(levels: LevelValue[]): string {
  return JSON.stringify(
    levels.map((level) =>
      level.type === 'dept_leader'
        ? { type: level.type, dept_level: level.deptLevel }
        : { type: level.type, user_ids: level.userIds }
    )
  )
}

// 指定审批人选择器:关键字搜索用户(管理端 /api/user/search),点选加入、徽标移除。
function DesignatedApproverPicker(props: {
  value: number[]
  onChange: (ids: number[]) => void
  disabled?: boolean
}) {
  const { t } = useTranslation()
  const [keyword, setKeyword] = useState('')
  const trimmedKeyword = keyword.trim()

  const searchQuery = useQuery({
    queryKey: ['recharge-approval-user-search', trimmedKeyword],
    queryFn: async () => {
      const res = await searchUsers({ keyword: trimmedKeyword, page_size: 5 })
      if (!res.success) throw new Error(res.message)
      return res.data?.items ?? []
    },
    enabled: trimmedKeyword.length > 0,
  })

  // 已选 id 的展示名:新选中的来自搜索结果,存量来自按 id 拉取。
  const idsKey = props.value.join(',')
  const namesQuery = useQuery({
    queryKey: ['recharge-approval-user-names', idsKey],
    queryFn: async () => {
      const names = new Map<number, string>()
      await Promise.all(
        props.value.map(async (id) => {
          const res = await getUser(id)
          if (res.success && res.data) {
            names.set(id, res.data.display_name || res.data.username)
          }
        })
      )
      return names
    },
    enabled: props.value.length > 0,
  })

  const results = useMemo(() => {
    const items = searchQuery.data ?? []
    return items.filter((user) => !props.value.includes(user.id))
  }, [searchQuery.data, props.value])

  const nameOf = (id: number) => namesQuery.data?.get(id) ?? `#${id}`

  const addUser = (id: number) => {
    if (!props.value.includes(id)) props.onChange([...props.value, id])
  }

  const removeUser = (id: number) => {
    props.onChange(props.value.filter((v) => v !== id))
  }

  let resultsContent: ReactNode = null
  if (trimmedKeyword.length > 0) {
    if (searchQuery.isLoading) {
      resultsContent = (
        <div className='text-muted-foreground px-3 py-2 text-xs'>
          {t('Loading...')}
        </div>
      )
    } else if (results.length === 0) {
      resultsContent = (
        <div className='text-muted-foreground px-3 py-2 text-xs'>
          {t('No matching users')}
        </div>
      )
    } else {
      resultsContent = (
        <ul>
          {results.map((user) => (
            <li key={user.id}>
              <button
                type='button'
                className='hover:bg-muted/60 flex w-full items-center justify-between px-3 py-2 text-left text-sm'
                onClick={() => addUser(user.id)}
              >
                <span>
                  {user.display_name || user.username}
                  <span className='text-muted-foreground ml-2 text-xs'>
                    @{user.username} · #{user.id}
                  </span>
                </span>
                <Plus className='size-3.5' aria-hidden='true' />
              </button>
            </li>
          ))}
        </ul>
      )
    }
  }

  return (
    <div className='space-y-2'>
      {props.value.length > 0 && (
        <div className='flex flex-wrap gap-1.5'>
          {props.value.map((id) => (
            <Badge key={id} variant='secondary' className='gap-1'>
              {nameOf(id)}
              <button
                type='button'
                aria-label={t('Remove')}
                className='hover:text-foreground text-muted-foreground'
                onClick={() => removeUser(id)}
                disabled={props.disabled}
              >
                <X className='size-3' aria-hidden='true' />
              </button>
            </Badge>
          ))}
        </div>
      )}
      <div className='relative'>
        <Search
          className='text-muted-foreground absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2'
          aria-hidden='true'
        />
        <Input
          value={keyword}
          onChange={(e) => setKeyword(e.target.value)}
          placeholder={t('Search users by keyword...')}
          className='h-9 pl-10'
          disabled={props.disabled}
        />
      </div>
      {trimmedKeyword.length > 0 && (
        <div className='rounded-md border'>{resultsContent}</div>
      )}
    </div>
  )
}

export function RechargeApprovalSection({
  defaultValues,
}: {
  defaultValues: {
    enabled: boolean
    quotaUsd: number
    levels: string
  }
}) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()

  const defaultLevels = useMemo(
    () => parseLevelsJson(defaultValues.levels),
    [defaultValues.levels]
  )

  const form = useForm<Values>({
    resolver: zodResolver(schema) as unknown as Resolver<Values>,
    defaultValues: {
      enabled: defaultValues.enabled,
      quotaUsd: defaultValues.quotaUsd,
      levels: defaultLevels,
    },
  })

  const { fields, append, remove } = useFieldArray({
    control: form.control,
    name: 'levels',
  })

  const { isDirty, isSubmitting } = form.formState
  const enabled = form.watch('enabled')
  const saving = updateOption.isPending || isSubmitting

  async function onSubmit(values: Values) {
    const updates: Array<{ key: string; value: string }> = []

    if (values.enabled !== defaultValues.enabled) {
      updates.push({
        key: 'recharge_approval.enabled',
        value: String(values.enabled),
      })
    }

    if (values.quotaUsd !== defaultValues.quotaUsd) {
      updates.push({
        key: 'recharge_approval.quota_usd',
        value: String(values.quotaUsd),
      })
    }

    const serialized = serializeLevels(values.levels)
    if (serialized !== serializeLevels(defaultLevels)) {
      updates.push({ key: 'recharge_approval.levels', value: serialized })
    }

    if (updates.length === 0) {
      toast.info(t('No changes to save'))
      return
    }

    // 后端按合并后整体校验:启用时先写额度与级别、最后写开关;
    // 停用时反过来先写开关,否则清空级别等中间态会被仍启用的旧配置拒绝。
    const enabling = values.enabled
    updates.sort((a) =>
      (a.key === 'recharge_approval.enabled') === enabling ? 1 : -1
    )

    for (const update of updates) {
      await updateOption.mutateAsync(update)
    }

    form.reset(values)
  }

  return (
    <SettingsSection title={t('Recharge Approval')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)} autoComplete='off'>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={saving}
            isSaveDisabled={!isDirty}
            saveLabel='Save recharge approval settings'
          />

          <FormField
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable recharge requests')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Allow users to submit fixed-amount recharge requests that require multi-level approval'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                    disabled={saving}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          <FormField
            control={form.control}
            name='quotaUsd'
            render={({ field }) => (
              <FormItem className='max-w-xs'>
                <FormLabel>{t('Fixed recharge amount (USD)')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    min={0.01}
                    step='0.01'
                    placeholder='10'
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t('Every approved request credits exactly this amount.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <div className='space-y-3'>
            <div>
              <div className='text-sm font-medium'>{t('Approval Levels')}</div>
              <p className='text-muted-foreground mt-0.5 text-xs'>
                {t(
                  'Levels are approved in order; any single approver at a level can pass it. Department-leader levels require organization structure sync; if a level cannot be resolved for a user, submission is blocked — consider adding a designated-approver level at the end as a fallback.'
                )}
              </p>
            </div>

            {fields.map((fieldItem, index) => {
              const levelType = form.watch(`levels.${index}.type`)
              return (
                <div
                  key={fieldItem.id}
                  className='space-y-3 rounded-lg border p-4'
                >
                  <div className='flex items-center justify-between gap-2'>
                    <span className='text-sm font-medium'>
                      {t('Level {{level}}', { level: index + 1 })}
                    </span>
                    <Button
                      type='button'
                      variant='ghost'
                      size='sm'
                      onClick={() => remove(index)}
                      disabled={saving}
                    >
                      <X data-icon='inline-start' aria-hidden='true' />
                      {t('Remove')}
                    </Button>
                  </div>

                  <FormField
                    control={form.control}
                    name={`levels.${index}.type`}
                    render={({ field }) => (
                      <FormItem className='max-w-xs'>
                        <FormLabel>{t('Approver Type')}</FormLabel>
                        <Select
                          value={field.value}
                          onValueChange={field.onChange}
                        >
                          <FormControl>
                            <SelectTrigger>
                              <SelectValue />
                            </SelectTrigger>
                          </FormControl>
                          <SelectContent alignItemWithTrigger={false}>
                            <SelectGroup>
                              <SelectItem value='dept_leader'>
                                {t('Department Leader')}
                              </SelectItem>
                              <SelectItem value='designated'>
                                {t('Designated Approver')}
                              </SelectItem>
                            </SelectGroup>
                          </SelectContent>
                        </Select>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  {levelType === 'dept_leader' && (
                    <FormField
                      control={form.control}
                      name={`levels.${index}.deptLevel`}
                      render={({ field }) => (
                        <FormItem className='max-w-xs'>
                          <FormLabel>{t('Department Level (1-5)')}</FormLabel>
                          <FormControl>
                            <Input
                              type='number'
                              min={1}
                              max={MAX_DEPT_LEVEL}
                              {...field}
                            />
                          </FormControl>
                          <FormDescription>
                            {t(
                              '1 = direct department leader, 2 = parent department leader, ...'
                            )}
                          </FormDescription>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                  )}

                  {levelType === 'designated' && (
                    <FormField
                      control={form.control}
                      name={`levels.${index}.userIds`}
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t('Designated Approvers')}</FormLabel>
                          <FormControl>
                            <DesignatedApproverPicker
                              value={field.value}
                              onChange={field.onChange}
                              disabled={saving}
                            />
                          </FormControl>
                          <FormDescription>
                            {t(
                              'Search and add one or more approvers for this level.'
                            )}
                          </FormDescription>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                  )}
                </div>
              )
            })}

            {fields.length < MAX_LEVELS && (
              <Button
                type='button'
                variant='outline'
                size='sm'
                onClick={() =>
                  append({ type: 'designated', deptLevel: 1, userIds: [] })
                }
                disabled={saving}
              >
                <Plus data-icon='inline-start' aria-hidden='true' />
                {t('Add Level')}
              </Button>
            )}
            {fields.length === 0 && enabled && (
              <p className='text-destructive text-xs'>
                {t(
                  'At least one approval level is required before enabling submissions.'
                )}
              </p>
            )}
          </div>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
