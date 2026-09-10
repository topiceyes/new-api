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
import { Ban, ShieldCheck, Trash2 } from 'lucide-react'
import { useEffect, useRef } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'

import { deleteRateLimitedIP, getRateLimitedIPs } from '../api'
import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import type { RateLimitedIPRecord } from '../types'

const ipAccessSchema = z.object({
  ip_access: z.object({
    whitelist: z.string(),
    blacklist: z.string(),
  }),
})

type IPAccessFormValues = z.output<typeof ipAccessSchema>
type IPAccessFormInput = z.input<typeof ipAccessSchema>

type NormalizedIPAccessValues = {
  'ip_access.whitelist': string[]
  'ip_access.blacklist': string[]
}

type IPAccessSectionProps = {
  defaultValues: NormalizedIPAccessValues
}

const splitLines = (value: string) =>
  value
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line.length > 0)

const buildFormDefaults = (
  defaults: NormalizedIPAccessValues
): IPAccessFormInput => ({
  ip_access: {
    whitelist: defaults['ip_access.whitelist'].join('\n'),
    blacklist: defaults['ip_access.blacklist'].join('\n'),
  },
})

const normalizeFormValues = (
  values: IPAccessFormValues
): NormalizedIPAccessValues => ({
  'ip_access.whitelist': splitLines(values.ip_access.whitelist),
  'ip_access.blacklist': splitLines(values.ip_access.blacklist),
})

const isEqual = (a: string[], b: string[]) =>
  JSON.stringify(a) === JSON.stringify(b)

const formatTimestamp = (ts: number) =>
  ts > 0 ? new Date(ts * 1000).toLocaleString() : '-'

export function IPAccessSection({ defaultValues }: IPAccessSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const queryClient = useQueryClient()
  const baselineRef = useRef<NormalizedIPAccessValues>(defaultValues)

  const form = useForm<IPAccessFormInput, unknown, IPAccessFormValues>({
    resolver: zodResolver(ipAccessSchema),
    mode: 'onChange',
    defaultValues: buildFormDefaults(defaultValues),
  })

  useEffect(() => {
    baselineRef.current = defaultValues
    form.reset(buildFormDefaults(defaultValues))
  }, [defaultValues, form])

  const recordsQuery = useQuery({
    queryKey: ['rate-limited-ips'],
    queryFn: getRateLimitedIPs,
  })
  const records: RateLimitedIPRecord[] = recordsQuery.data?.data ?? []

  const deleteRecord = useMutation({
    mutationFn: deleteRateLimitedIP,
    onSuccess: (data) => {
      if (data.success) {
        queryClient.invalidateQueries({ queryKey: ['rate-limited-ips'] })
        toast.success(t('Record removed'))
      } else {
        toast.error(data.message || t('Failed to remove record'))
      }
    },
    onError: (error: Error) => {
      toast.error(error.message || t('Failed to remove record'))
    },
  })

  const saveList = async (
    key: keyof NormalizedIPAccessValues,
    entries: string[]
  ) => {
    await updateOption.mutateAsync({ key, value: JSON.stringify(entries) })
  }

  const onSubmit = async (values: IPAccessFormValues) => {
    const normalized = normalizeFormValues(values)
    const updates = (
      Object.keys(normalized) as Array<keyof NormalizedIPAccessValues>
    ).filter((key) => !isEqual(normalized[key], baselineRef.current[key]))

    if (updates.length === 0) {
      toast.info(t('No changes to save'))
      return
    }

    for (const key of updates) {
      await saveList(key, normalized[key])
    }
    baselineRef.current = normalized
  }

  // 一键加白/加黑:基于文本框当前内容(含未保存的编辑)追加,避免保存后
  // 表单随 options 刷新而重置、丢掉另一份名单里未保存的修改。
  const addToList = async (
    record: RateLimitedIPRecord,
    key: keyof NormalizedIPAccessValues
  ) => {
    const draft = splitLines(form.getValues(key))
    if (draft.includes(record.ip)) {
      toast.info(t('This IP is already in the list'))
      return
    }
    await saveList(key, [...draft, record.ip])
    await deleteRecord.mutateAsync(record.ip)
  }

  return (
    <SettingsSection title={t('IP Access Control')}>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Whitelisted IPs skip all IP-based rate limits; blacklisted IPs are blocked from the entire site. One IP or CIDR range per line.'
        )}
      </p>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save IP access control'
          />
          <FormField
            control={form.control}
            name='ip_access.whitelist'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('IP Whitelist')}</FormLabel>
                <FormControl>
                  <Textarea
                    rows={4}
                    placeholder={'203.0.113.10\n198.51.100.0/24'}
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'IPs and CIDR ranges that bypass all IP-based rate limits (login, global web/API, etc.). One entry per line.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='ip_access.blacklist'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('IP Blacklist')}</FormLabel>
                <FormControl>
                  <Textarea
                    rows={4}
                    placeholder={'192.0.2.55\n192.0.2.0/24'}
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'IPs and CIDR ranges permanently blocked from the entire site with 403. One entry per line.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>

      <div className='space-y-2'>
        <div className='flex items-center justify-between'>
          <h3 className='text-sm font-medium'>
            {t('Rate-limited IP Records')}
          </h3>
          <Button
            variant='outline'
            size='sm'
            onClick={() => recordsQuery.refetch()}
            disabled={recordsQuery.isFetching}
          >
            {t('Refresh')}
          </Button>
        </div>
        <p className='text-muted-foreground text-sm'>
          {t(
            'IPs that recently hit a rate limit (429). Add them to the whitelist or blacklist with one click.'
          )}
        </p>
        <div className='rounded-md border'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('IP')}</TableHead>
                <TableHead>{t('Hits')}</TableHead>
                <TableHead>{t('Rate limit buckets')}</TableHead>
                <TableHead>{t('First seen')}</TableHead>
                <TableHead>{t('Last seen')}</TableHead>
                <TableHead className='text-right'>{t('Actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {records.length === 0 ? (
                <TableRow>
                  <TableCell
                    colSpan={6}
                    className='text-muted-foreground text-center'
                  >
                    {recordsQuery.isLoading
                      ? t('Loading...')
                      : t('No rate-limited IPs recorded')}
                  </TableCell>
                </TableRow>
              ) : (
                records.map((record) => (
                  <TableRow key={record.ip}>
                    <TableCell className='font-mono'>{record.ip}</TableCell>
                    <TableCell>{record.hits}</TableCell>
                    <TableCell>
                      <div className='flex flex-wrap gap-1'>
                        {record.marks.map((mark) => (
                          <Badge key={mark} variant='secondary'>
                            {mark}
                          </Badge>
                        ))}
                      </div>
                    </TableCell>
                    <TableCell>{formatTimestamp(record.first_seen)}</TableCell>
                    <TableCell>{formatTimestamp(record.last_seen)}</TableCell>
                    <TableCell className='text-right'>
                      <div className='flex justify-end gap-1'>
                        <Button
                          variant='outline'
                          size='sm'
                          onClick={() =>
                            addToList(record, 'ip_access.whitelist')
                          }
                          disabled={updateOption.isPending}
                        >
                          <ShieldCheck className='mr-1 h-3.5 w-3.5' />
                          {t('Whitelist')}
                        </Button>
                        <Button
                          variant='outline'
                          size='sm'
                          onClick={() =>
                            addToList(record, 'ip_access.blacklist')
                          }
                          disabled={updateOption.isPending}
                        >
                          <Ban className='mr-1 h-3.5 w-3.5' />
                          {t('Blacklist')}
                        </Button>
                        <Button
                          variant='ghost'
                          size='sm'
                          onClick={() => deleteRecord.mutate(record.ip)}
                          disabled={deleteRecord.isPending}
                        >
                          <Trash2 className='h-3.5 w-3.5' />
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>
      </div>
    </SettingsSection>
  )
}
