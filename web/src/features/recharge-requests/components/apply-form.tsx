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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CircleAlert } from 'lucide-react'
import { useMemo } from 'react'
import { useForm, type Resolver } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Textarea } from '@/components/ui/textarea'
import { formatQuotaWithCurrency } from '@/lib/currency'

import { getRechargeConfig, submitRechargeRequest } from '../api'
import { CATEGORY_LABEL_KEY, LEVEL_TYPE_LABEL_KEY } from '../constants'
import type { RechargeCategory } from '../types'

const SKELETON_KEYS = ['sk-1', 'sk-2', 'sk-3']

const DETAIL_MIN_LENGTH = 10
const DETAIL_MAX_LENGTH = 500

const buildSchema = (t: (key: string) => string) =>
  z.object({
    category: z.enum(['project_delivery', 'tech_research']),
    detail: z
      .string()
      .trim()
      .min(DETAIL_MIN_LENGTH, {
        message: t('Please describe in at least 10 characters'),
      })
      .max(DETAIL_MAX_LENGTH, {
        message: t('Up to 500 characters.'),
      }),
  })

type Values = z.infer<ReturnType<typeof buildSchema>>

export function ApplyForm() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const configQuery = useQuery({
    queryKey: ['recharge-config'],
    queryFn: async () => {
      const res = await getRechargeConfig()
      if (!res.success) throw new Error(res.message)
      return res.data
    },
  })
  const config = configQuery.data

  const schema = useMemo(() => buildSchema(t), [t])

  const form = useForm<Values>({
    resolver: zodResolver(schema) as unknown as Resolver<Values>,
    defaultValues: {
      category: 'project_delivery',
      detail: '',
    },
  })

  const submitMutation = useMutation({
    mutationFn: (values: Values) => submitRechargeRequest(values),
    onSuccess: (res) => {
      if (!res.success) {
        toast.error(res.message || t('Submission failed, please try again'))
        return
      }
      toast.success(t('Recharge request submitted'))
      form.reset()
      queryClient.invalidateQueries({ queryKey: ['recharge-my-requests'] })
    },
    onError: (error: Error) => {
      toast.error(error.message || t('Submission failed, please try again'))
    },
  })

  if (configQuery.isLoading) {
    return (
      <div className='space-y-3'>
        {SKELETON_KEYS.map((key) => (
          <Skeleton key={key} className='h-24 w-full rounded-lg' />
        ))}
      </div>
    )
  }

  if (!config || !config.enabled) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>{t('New Request')}</CardTitle>
          <CardDescription>
            {t(
              'Recharge requests are currently disabled. Please contact an administrator.'
            )}
          </CardDescription>
        </CardHeader>
      </Card>
    )
  }

  const canSubmit = config.resolvable && !submitMutation.isPending

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('New Request')}</CardTitle>
        <CardDescription>
          {t(
            'Submit a recharge request with a fixed amount. It takes effect after all approval levels pass.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='space-y-6'>
        {!config.resolvable && (
          <Alert variant='destructive'>
            <CircleAlert aria-hidden='true' />
            <AlertTitle>{t('Unable to submit right now')}</AlertTitle>
            <AlertDescription>
              {config.resolve_error ||
                t('The approval chain could not be resolved for your account.')}
            </AlertDescription>
          </Alert>
        )}

        <div className='rounded-lg border p-4'>
          <div className='text-muted-foreground text-xs'>
            {t('Recharge Amount')}
          </div>
          <div className='mt-1 text-lg font-semibold'>
            {formatQuotaWithCurrency(config.quota)}
          </div>
          <p className='text-muted-foreground mt-1 text-xs'>
            {t('The amount is fixed by the administrator for every request.')}
          </p>
        </div>

        <Form {...form}>
          <form
            onSubmit={form.handleSubmit((values) =>
              submitMutation.mutate(values)
            )}
            className='space-y-4'
          >
            <FormField
              control={form.control}
              name='category'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Category')}</FormLabel>
                  <Select
                    value={field.value}
                    onValueChange={(value) =>
                      field.onChange(value as RechargeCategory)
                    }
                  >
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent alignItemWithTrigger={false}>
                      <SelectGroup>
                        {config.categories.map((category) => (
                          <SelectItem key={category} value={category}>
                            {t(CATEGORY_LABEL_KEY[category])}
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
              name='detail'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Details')}</FormLabel>
                  <FormControl>
                    <Textarea
                      maxLength={500}
                      rows={4}
                      placeholder={t(
                        'Describe the purpose, related project, and expected usage...'
                      )}
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('At least 10 characters, up to 500 characters.')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            {config.chain.length > 0 && (
              <div className='rounded-lg border p-4'>
                <div className='mb-2 text-sm font-medium'>
                  {t('Approval Chain')}
                </div>
                <ol className='space-y-1.5'>
                  {config.chain.map((level) => (
                    <li
                      key={level.level}
                      className='text-muted-foreground text-sm'
                    >
                      {t('Level {{level}}', { level: level.level })} ·{' '}
                      {t(LEVEL_TYPE_LABEL_KEY[level.type])} ·{' '}
                      {level.approvers.map((a) => a.name).join(', ')}
                    </li>
                  ))}
                </ol>
              </div>
            )}

            <Button type='submit' disabled={!canSubmit}>
              {submitMutation.isPending ? t('Submitting...') : t('Submit')}
            </Button>
          </form>
        </Form>
      </CardContent>
    </Card>
  )
}
