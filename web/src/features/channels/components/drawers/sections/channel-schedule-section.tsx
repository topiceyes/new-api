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
import { Clock, Plus, Trash2 } from 'lucide-react'
import { useFieldArray, useFormContext, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { IconBadge } from '@/components/ui/icon-badge'
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
import { cn } from '@/lib/utils'

import type { ChannelFormValues } from '@/features/channels/lib'

const DAY_KEYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'] as const

const FALLBACK_TIMEZONES = [
  'Asia/Shanghai',
  'UTC',
  'America/New_York',
  'Europe/London',
  'Asia/Tokyo',
]

function getTimezoneOptions(): string[] {
  try {
    if (typeof Intl.supportedValuesOf === 'function') {
      return Intl.supportedValuesOf('timeZone')
    }
  } catch {
    // not supported in this runtime
  }
  return FALLBACK_TIMEZONES
}

export function ChannelScheduleSection(props: {
  id?: string
  className?: string
}) {
  const { t } = useTranslation()
  const { control } = useFormContext<ChannelFormValues>()
  const { fields, append, remove, update } = useFieldArray({
    control,
    name: 'schedule_windows',
  })
  const scheduleEnabled = useWatch({ control, name: 'schedule_enabled' })

  return (
    <div
      id={props.id}
      className={cn('border-border/60 rounded-lg border p-3', props.className)}
    >
      <div className='flex items-center gap-3'>
        <IconBadge tone='info' size='md'>
          <Clock className='h-4 w-4' aria-hidden='true' />
        </IconBadge>
        <h3 className='text-sm font-semibold tracking-tight'>
          {t('Scheduled Switch')}
        </h3>
      </div>

      <div className='mt-4 flex flex-col gap-4'>
        <FormField
          control={control}
          name='schedule_enabled'
          render={({ field }) => (
            <FormItem className='flex items-center justify-between'>
              <div className='space-y-0.5'>
                <FormLabel>{t('Enable scheduled switching')}</FormLabel>
                <FormDescription>
                  {t(
                    'Channel runs only inside the configured windows; outside them it is auto-disabled. Overnight windows like 22:00-02:00 belong to the start day.'
                  )}
                </FormDescription>
              </div>
              <FormControl>
                <Switch
                  checked={field.value === true}
                  onCheckedChange={field.onChange}
                />
              </FormControl>
            </FormItem>
          )}
        />

        {scheduleEnabled && (
          <>
            <FormField
              control={control}
              name='schedule_timezone'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Timezone')}</FormLabel>
                  <Select onValueChange={field.onChange} value={field.value}>
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue placeholder={t('Timezone')} />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      <SelectGroup>
                        {getTimezoneOptions().map((tz) => (
                          <SelectItem key={tz} value={tz}>
                            {tz}
                          </SelectItem>
                        ))}
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                  <FormMessage />
                </FormItem>
              )}
            />

            <div className='flex flex-col gap-3'>
              <FormLabel>{t('Schedule windows')}</FormLabel>

              {fields.map((item, index) => (
                <div
                  key={item.id}
                  className='border-border/60 flex flex-col gap-3 rounded-md border p-3'
                >
                  <div className='flex flex-wrap items-center gap-1'>
                    {DAY_KEYS.map((day, dayIndex) => {
                      const selected = item.days?.includes(dayIndex) ?? false
                      return (
                        <button
                          key={day}
                          type='button'
                          onClick={() => {
                            const current = item.days || []
                            const next = current.includes(dayIndex)
                              ? current.filter((d) => d !== dayIndex)
                              : [...current, dayIndex].sort((a, b) => a - b)
                            update(index, { ...item, days: next })
                          }}
                          className={cn(
                            'rounded-md border px-2 py-1 text-xs transition-colors',
                            selected
                              ? 'bg-primary text-primary-foreground border-primary'
                              : 'bg-background hover:bg-muted text-muted-foreground'
                          )}
                        >
                          {t(day)}
                        </button>
                      )
                    })}
                    <button
                      type='button'
                      onClick={() => {
                        const allDays = [0, 1, 2, 3, 4, 5, 6]
                        const current = item.days || []
                        const isEveryDay =
                          current.length === 7 &&
                          allDays.every((d) => current.includes(d))
                        update(index, {
                          ...item,
                          days: isEveryDay ? [] : allDays,
                        })
                      }}
                      className='text-muted-foreground hover:text-foreground ml-auto rounded-md border px-2 py-1 text-xs transition-colors'
                    >
                      {t('Every day')}
                    </button>
                  </div>

                  <div className='grid grid-cols-2 gap-3'>
                    <FormField
                      control={control}
                      name={`schedule_windows.${index}.start`}
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t('Start time')}</FormLabel>
                          <FormControl>
                            <Input type='time' {...field} />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                    <FormField
                      control={control}
                      name={`schedule_windows.${index}.end`}
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t('End time')}</FormLabel>
                          <FormControl>
                            <Input type='time' {...field} />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                  </div>

                  <div className='flex justify-end'>
                    <Button
                      type='button'
                      variant='ghost'
                      size='sm'
                      onClick={() => remove(index)}
                      className='text-destructive hover:text-destructive'
                    >
                      <Trash2 className='mr-1 h-3.5 w-3.5' aria-hidden='true' />
                      {t('Delete')}
                    </Button>
                  </div>
                </div>
              ))}

              <Button
                type='button'
                variant='outline'
                size='sm'
                onClick={() =>
                  append({ days: [1, 2, 3, 4, 5], start: '00:30', end: '08:30' })
                }
                className='w-full'
              >
                <Plus className='mr-1 h-3.5 w-3.5' aria-hidden='true' />
                {t('Add window')}
              </Button>
            </div>
          </>
        )}
      </div>
    </div>
  )
}
