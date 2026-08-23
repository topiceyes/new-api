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
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  FormControl,
  FormDescription,
  FormField,
  FormLabel,
} from '@/components/ui/form'
import { Switch } from '@/components/ui/switch'

import { testFeishuNotify } from '../api'
import { SettingsSwitchContent, SettingsSwitchItem } from '../components/settings-form-layout'

import type { OAuthFormValues } from './oauth-section'
import type { UseFormReturn } from 'react-hook-form'

interface Props {
  form: UseFormReturn<OAuthFormValues>
}

export function FeishuNotifyBlock({ form }: Props) {
  const { t } = useTranslation()
  const testMutation = useMutation({
    mutationFn: testFeishuNotify,
    onSuccess: (data) => {
      if (data.success) {
        toast.success(data.message || t('Test message sent'))
      } else {
        toast.error(data.message || t('Failed to send test message'))
      }
    },
    onError: () => {
      toast.error(t('Failed to send test message'))
    },
  })

  return (
    <div className='space-y-6 lg:col-span-2'>
      <div className='border-border mt-6 border-t pt-6'>
        <h4 className='mb-4 text-sm font-medium'>{t('Message Notifications')}</h4>
        <div className='grid gap-x-5 gap-y-6 lg:grid-cols-2'>
          <FormField
            control={form.control}
            name='feishu.notify_enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable Feishu Message Notifications')}</FormLabel>
                  <FormDescription>
                    {t('Send plan usage alerts to root user via Feishu IM')}
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
        </div>

        <div className='mt-4'>
          <Button
            type='button'
            variant='outline'
            onClick={() => testMutation.mutate()}
            disabled={testMutation.isPending}
          >
            {testMutation.isPending ? t('Sending...') : t('Send Test Message')}
          </Button>
          <FormDescription className='mt-2'>
            {t('Test uses saved settings; save changes before testing')}
          </FormDescription>
        </div>
      </div>
    </div>
  )
}
