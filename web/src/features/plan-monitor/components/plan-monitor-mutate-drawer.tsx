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
import { useQueryClient } from '@tanstack/react-query'
import { Settings2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useForm, type Resolver } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import {
  SideDrawerSection,
  sideDrawerContentClassName,
  sideDrawerFooterClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
  sideDrawerSwitchItemClassName,
} from '@/components/drawer-layout'
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
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Switch } from '@/components/ui/switch'

import { createPlan, listPlans, updatePlan } from '../api'
import { providerLabel } from '../lib'
import type { PlanMonitorPayload, PlanMonitorPlan } from '../types'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentRow?: PlanMonitorPlan
}

interface PlanMonitorFormValues {
  provider: string
  plan_name: string
  api_url: string
  api_key: string
  refresh_interval_min: number
  sort_order: number
  enabled: boolean
}

export function PlanMonitorMutateDrawer({
  open,
  onOpenChange,
  currentRow,
}: Props) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isEdit = !!currentRow?.id
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [providers, setProviders] = useState<string[]>([])

  const schema = z.object({
    provider: z.string().min(1, t('Please select a provider')),
    plan_name: z.string().min(1, t('Please enter plan name')),
    api_url: z.string(),
    api_key: isEdit
      ? z.string()
      : z.string().min(1, t('Please enter API key')),
    refresh_interval_min: z.coerce
      .number()
      .min(1, t('Refresh interval must be greater than 0')),
    sort_order: z.coerce.number().int(),
    enabled: z.boolean(),
  })

  const form = useForm<PlanMonitorFormValues>({
    resolver: zodResolver(schema) as unknown as Resolver<PlanMonitorFormValues>,
    defaultValues: {
      provider: '',
      plan_name: '',
      api_url: '',
      api_key: '',
      refresh_interval_min: 5,
      sort_order: 0,
      enabled: true,
    },
  })

  useEffect(() => {
    if (!open) return

    if (currentRow) {
      form.reset({
        provider: currentRow.provider,
        plan_name: currentRow.plan_name,
        api_url: currentRow.api_url,
        api_key: '',
        refresh_interval_min: currentRow.refresh_interval_min || 5,
        sort_order: currentRow.sort_order || 0,
        enabled: currentRow.enabled,
      })
    } else {
      form.reset({
        provider: '',
        plan_name: '',
        api_url: '',
        api_key: '',
        refresh_interval_min: 5,
        sort_order: 0,
        enabled: true,
      })
    }

    listPlans()
      .then((res) => {
        if (res.success) setProviders(res.data.supported_providers || [])
      })
      .catch(() => {})
  }, [open, currentRow, form])

  const onSubmit = async (values: PlanMonitorFormValues) => {
    setIsSubmitting(true)
    try {
      const payload: PlanMonitorPayload = {
        provider: values.provider,
        plan_name: values.plan_name,
        api_url: values.api_url,
        api_key: values.api_key,
        refresh_interval_min: Number(values.refresh_interval_min || 5),
        sort_order: Number(values.sort_order || 0),
        enabled: values.enabled,
      }
      const res =
        isEdit && currentRow?.id
          ? await updatePlan(currentRow.id, payload)
          : await createPlan(payload)
      if (res.success) {
        toast.success(isEdit ? t('Update succeeded') : t('Create succeeded'))
        onOpenChange(false)
        queryClient.invalidateQueries({ queryKey: ['plan-monitor-plans'] })
        queryClient.invalidateQueries({ queryKey: ['plan-monitor-overview'] })
      } else {
        toast.error(res.message || t('Operation failed'))
      }
    } catch {
      toast.error(t('Request failed'))
    } finally {
      setIsSubmitting(false)
    }
  }

  // Ensure the current provider is selectable even when the supported list
  // has not loaded yet.
  const providerOptions =
    currentRow && !providers.includes(currentRow.provider)
      ? [currentRow.provider, ...providers]
      : providers

  return (
    <Sheet
      open={open}
      onOpenChange={(v) => {
        onOpenChange(v)
        if (!v) {
          form.reset()
        }
      }}
    >
      <SheetContent className={sideDrawerContentClassName('sm:max-w-[520px]')}>
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>
            {isEdit ? t('Update plan monitor') : t('Add plan monitor')}
          </SheetTitle>
          <SheetDescription>
            {isEdit
              ? t('Modify the existing plan monitor configuration')
              : t('Fill in the following info to monitor an upstream plan')}
          </SheetDescription>
        </SheetHeader>
        <Form {...form}>
          <form
            id='plan-monitor-form'
            onSubmit={form.handleSubmit(onSubmit)}
            className={sideDrawerFormClassName()}
          >
            <SideDrawerSection>
              <h3 className='flex items-center gap-2 text-sm font-medium'>
                <IconBadge tone='info' size='xs'>
                  <Settings2 />
                </IconBadge>
                {t('Basic Info')}
              </h3>

              <FormField
                control={form.control}
                name='provider'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Provider')}</FormLabel>
                    <Select
                      items={providerOptions.map((p) => ({
                        value: p,
                        label: providerLabel(p),
                      }))}
                      onValueChange={field.onChange}
                      value={field.value || ''}
                      disabled={isEdit}
                    >
                      <FormControl>
                        <SelectTrigger className='w-full'>
                          <SelectValue
                            placeholder={t('Select a provider')}
                          />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent alignItemWithTrigger={false}>
                        <SelectGroup>
                          {providerOptions.map((p) => (
                            <SelectItem key={p} value={p}>
                              {providerLabel(p)}
                            </SelectItem>
                          ))}
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='plan_name'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Plan Name')}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        placeholder={t('e.g. MiniMax Coding Plan')}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='api_url'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('API URL')}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        placeholder={t('Leave blank to use the default')}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='api_key'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('API Key')}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        type='password'
                        autoComplete='off'
                        placeholder={
                          isEdit
                            ? currentRow?.api_key_masked ||
                              t('Leave empty to keep unchanged')
                            : t('Enter API key')
                        }
                      />
                    </FormControl>
                    {isEdit && (
                      <FormDescription>
                        {t('Leave empty to keep the current API key unchanged')}
                      </FormDescription>
                    )}
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='refresh_interval_min'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Refresh Interval (minutes)')}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        type='number'
                        min={1}
                        onChange={(e) =>
                          field.onChange(
                            Number.parseInt(e.target.value, 10) || 0
                          )
                        }
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='sort_order'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Sort Order')}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        type='number'
                        onChange={(e) =>
                          field.onChange(
                            Number.parseInt(e.target.value, 10) || 0
                          )
                        }
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Smaller values appear first')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='enabled'
                render={({ field }) => (
                  <FormItem className={sideDrawerSwitchItemClassName()}>
                    <FormLabel className='!mt-0'>
                      {t('Enabled Status')}
                    </FormLabel>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                  </FormItem>
                )}
              />
            </SideDrawerSection>
          </form>
        </Form>
        <SheetFooter className={sideDrawerFooterClassName()}>
          <SheetClose render={<Button variant='outline' />}>
            {t('Close')}
          </SheetClose>
          <Button
            form='plan-monitor-form'
            type='submit'
            disabled={isSubmitting}
          >
            {isSubmitting ? t('Saving...') : t('Save changes')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
