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
import { useMutation } from '@tanstack/react-query'
import { Loader2, RefreshCcw } from 'lucide-react'
import type { UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
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

import { runFeishuLeaveCheck } from '../api'
import {
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { safeNumberFieldProps } from '../utils/numeric-field'
import type { OAuthFormValues } from './oauth-section'

type FeishuPatrolBlockProps = {
  form: UseFormReturn<OAuthFormValues>
}

export function FeishuPatrolBlock(props: FeishuPatrolBlockProps) {
  const { t } = useTranslation()
  const patrolEnabled = props.form.watch('feishu.patrol_enabled')
  const patrolMode = props.form.watch('feishu.patrol_mode')

  const runCheck = useMutation({
    mutationFn: runFeishuLeaveCheck,
  })

  const handleRunNow = async () => {
    try {
      const res = await runCheck.mutateAsync()
      if (res.success && res.data) {
        toast.success(
          t(
            'Departure check finished: {{checked}} checked, {{disabled}} disabled, {{unknown}} unknown',
            {
              checked: res.data.checked,
              disabled: res.data.disabled,
              unknown: res.data.unknown,
            }
          )
        )
      } else {
        toast.error(
          res.message ||
            t('Departure check failed, please try again later')
        )
      }
    } catch {
      toast.error(t('Departure check failed, please try again later'))
    }
  }

  return (
    <>
      <div className='lg:col-span-2 border-t pt-5 text-sm font-medium'>
        {t('Departure patrol')}
      </div>

      <FormField
        control={props.form.control}
        name='feishu.patrol_enabled'
        render={({ field }) => (
          <SettingsSwitchItem>
            <SettingsSwitchContent>
              <FormLabel>{t('Enable departure patrol')}</FormLabel>
              <FormDescription>
                {t(
                  'Periodically check Feishu-bound users and automatically disable the account and API tokens of employees who left the organization'
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

      {patrolEnabled ? (
        <>
          <FormField
            control={props.form.control}
            name='feishu.patrol_mode'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Patrol schedule')}</FormLabel>
                <Select
                  items={[
                    { value: 'daily', label: t('Daily at a fixed hour') },
                    { value: 'interval', label: t('Every N hours') },
                  ]}
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
                      <SelectItem value='daily'>
                        {t('Daily at a fixed hour')}
                      </SelectItem>
                      <SelectItem value='interval'>
                        {t('Every N hours')}
                      </SelectItem>
                    </SelectGroup>
                  </SelectContent>
                </Select>
                <FormDescription>
                  {t(
                    'Departed employees are only disabled when Feishu explicitly confirms they left; unknown states are skipped'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          {patrolMode === 'daily' ? (
            <FormField
              control={props.form.control}
              name='feishu.patrol_hour'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Patrol hour (0-23)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      max={23}
                      step={1}
                      {...safeNumberFieldProps(field)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'The patrol runs once per day at or after this local hour'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          ) : (
            <FormField
              control={props.form.control}
              name='feishu.patrol_interval_hours'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Patrol interval (hours, 1-24)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      max={24}
                      step={1}
                      {...safeNumberFieldProps(field)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('How often the patrol runs, counted from the last run')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          )}

          <div className='lg:col-span-2 flex items-center gap-3'>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={handleRunNow}
              disabled={runCheck.isPending}
            >
              {runCheck.isPending ? (
                <Loader2 className='mr-1.5 h-3.5 w-3.5 animate-spin' />
              ) : (
                <RefreshCcw className='mr-1.5 h-3.5 w-3.5' />
              )}
              {runCheck.isPending ? t('Checking...') : t('Check now')}
            </Button>
            <span className='text-xs text-muted-foreground'>
              {t('Runs one patrol immediately and reports the result')}
            </span>
          </div>
        </>
      ) : null}
    </>
  )
}
