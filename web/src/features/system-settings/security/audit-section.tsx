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
import { useEffect, useMemo, useRef } from 'react'
import { useForm, type Resolver } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

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
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

const auditRuleSchema = z.object({
  id: z.string().min(1),
  name: z.string(),
  regex: z.string().min(1),
  severity: z.enum(['info', 'warning', 'critical']),
  enabled: z.boolean(),
})

const groupPromptPolicySchema = z.object({
  group: z.string().min(1),
  mode: z.enum(['none', 'hits', 'all']),
})

const auditSchema = z.object({
  audit: z.object({
    enabled: z.boolean(),
    prompt_scan_enabled: z.boolean(),
    store_prompt_mode: z.enum(['none', 'hits', 'all']),
    max_scan_bytes: z.coerce.number().int().min(1024).max(1048576),
    alert_enabled: z.boolean(),
    retention_days: z.coerce.number().int().min(0).max(3650),
    key_share_enabled: z.boolean(),
    key_share_window_minutes: z.coerce.number().int().min(1),
    key_share_distinct_ip_threshold: z.coerce.number().int().min(2),
    key_share_rapid_window_minutes: z.coerce.number().int().min(1),
    key_share_rapid_ip_threshold: z.coerce.number().int().min(2),
    key_share_suppress_hours: z.coerce.number().int().min(0),
    response_scan_enabled: z.boolean(),
    response_max_scan_bytes: z.coerce.number().int().min(1024).max(1048576),
    geoip_db_path: z.string(),
    classify_enabled: z.boolean(),
    classify_channel_id: z.coerce.number().int().min(0),
    classify_model: z.string(),
    classify_interval_minutes: z.coerce.number().int().min(1),
    classify_batch_size: z.coerce.number().int().min(1).max(100),
    // 自定义规则在表单里以 JSON 文本编辑,提交前按 schema 校验。
    rules: z.string(),
    // 分组级存储策略同样以 JSON 文本编辑。
    group_store_prompt_modes: z.string(),
  }),
})

type AuditFormValues = z.output<typeof auditSchema>

type AuditRule = z.output<typeof auditRuleSchema>

type NormalizedAuditValues = {
  'audit.enabled': boolean
  'audit.prompt_scan_enabled': boolean
  'audit.store_prompt_mode': string
  'audit.max_scan_bytes': number
  'audit.alert_enabled': boolean
  'audit.retention_days': number
  'audit.key_share_enabled': boolean
  'audit.key_share_window_minutes': number
  'audit.key_share_distinct_ip_threshold': number
  'audit.key_share_rapid_window_minutes': number
  'audit.key_share_rapid_ip_threshold': number
  'audit.key_share_suppress_hours': number
  'audit.response_scan_enabled': boolean
  'audit.response_max_scan_bytes': number
  'audit.geoip_db_path': string
  'audit.classify_enabled': boolean
  'audit.classify_channel_id': number
  'audit.classify_model': string
  'audit.classify_interval_minutes': number
  'audit.classify_batch_size': number
  'audit.rules': string
  'audit.group_store_prompt_modes': string
}

type AuditSectionProps = {
  defaultValues: Omit<
    NormalizedAuditValues,
    'audit.rules' | 'audit.group_store_prompt_modes'
  > & {
    // rules 与分组策略以原始 JSON 对象传入(后端存的是 JSON 数组),表单内转文本编辑。
    'audit.rules': unknown[]
    'audit.group_store_prompt_modes': unknown[]
  }
}

const buildFormDefaults = (
  defaults: AuditSectionProps['defaultValues']
): AuditFormValues => ({
  audit: {
    enabled: defaults['audit.enabled'],
    prompt_scan_enabled: defaults['audit.prompt_scan_enabled'],
    store_prompt_mode: defaults['audit.store_prompt_mode'] as
      | 'none'
      | 'hits'
      | 'all',
    max_scan_bytes: defaults['audit.max_scan_bytes'],
    alert_enabled: defaults['audit.alert_enabled'],
    retention_days: defaults['audit.retention_days'],
    key_share_enabled: defaults['audit.key_share_enabled'],
    key_share_window_minutes: defaults['audit.key_share_window_minutes'],
    key_share_distinct_ip_threshold:
      defaults['audit.key_share_distinct_ip_threshold'],
    key_share_rapid_window_minutes:
      defaults['audit.key_share_rapid_window_minutes'],
    key_share_rapid_ip_threshold: defaults['audit.key_share_rapid_ip_threshold'],
    key_share_suppress_hours: defaults['audit.key_share_suppress_hours'],
    response_scan_enabled: defaults['audit.response_scan_enabled'],
    response_max_scan_bytes: defaults['audit.response_max_scan_bytes'],
    geoip_db_path: defaults['audit.geoip_db_path'],
    classify_enabled: defaults['audit.classify_enabled'],
    classify_channel_id: defaults['audit.classify_channel_id'],
    classify_model: defaults['audit.classify_model'],
    classify_interval_minutes: defaults['audit.classify_interval_minutes'],
    classify_batch_size: defaults['audit.classify_batch_size'],
    rules: JSON.stringify(defaults['audit.rules'] ?? [], null, 2),
    group_store_prompt_modes: JSON.stringify(
      defaults['audit.group_store_prompt_modes'] ?? [],
      null,
      2
    ),
  },
})

const normalizeFormValues = (values: AuditFormValues): NormalizedAuditValues => ({
  'audit.enabled': values.audit.enabled,
  'audit.prompt_scan_enabled': values.audit.prompt_scan_enabled,
  'audit.store_prompt_mode': values.audit.store_prompt_mode,
  'audit.max_scan_bytes': values.audit.max_scan_bytes,
  'audit.alert_enabled': values.audit.alert_enabled,
  'audit.retention_days': values.audit.retention_days,
  'audit.key_share_enabled': values.audit.key_share_enabled,
  'audit.key_share_window_minutes': values.audit.key_share_window_minutes,
  'audit.key_share_distinct_ip_threshold':
    values.audit.key_share_distinct_ip_threshold,
  'audit.key_share_rapid_window_minutes':
    values.audit.key_share_rapid_window_minutes,
  'audit.key_share_rapid_ip_threshold': values.audit.key_share_rapid_ip_threshold,
  'audit.key_share_suppress_hours': values.audit.key_share_suppress_hours,
  'audit.response_scan_enabled': values.audit.response_scan_enabled,
  'audit.response_max_scan_bytes': values.audit.response_max_scan_bytes,
  'audit.geoip_db_path': values.audit.geoip_db_path,
  'audit.classify_enabled': values.audit.classify_enabled,
  'audit.classify_channel_id': values.audit.classify_channel_id,
  'audit.classify_model': values.audit.classify_model,
  'audit.classify_interval_minutes': values.audit.classify_interval_minutes,
  'audit.classify_batch_size': values.audit.classify_batch_size,
  'audit.rules': values.audit.rules,
  'audit.group_store_prompt_modes': values.audit.group_store_prompt_modes,
})

export function AuditSection({ defaultValues }: AuditSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const baselineRef = useRef<NormalizedAuditValues>(
    normalizeFormValues(buildFormDefaults(defaultValues))
  )

  const formDefaults = useMemo(
    () => buildFormDefaults(defaultValues),
    [defaultValues]
  )

  // schema 的数字字段用 z.coerce(输入态 unknown),与表单值类型(output)不同,
  // 这里按输出类型断言 resolver——校验时 coerce 自然处理字符串输入。
  const form = useForm<AuditFormValues>({
    resolver: zodResolver(auditSchema) as Resolver<AuditFormValues>,
    defaultValues: formDefaults,
  })

  useEffect(() => {
    const next = buildFormDefaults(defaultValues)
    baselineRef.current = normalizeFormValues(next)
    form.reset(next)
  }, [defaultValues, form])

  const onSubmit = async (data: AuditFormValues) => {
    // 校验自定义规则 JSON
    let rules: AuditRule[] = []
    const trimmed = data.audit.rules.trim()
    if (trimmed !== '' && trimmed !== '[]') {
      try {
        const parsed: unknown = JSON.parse(trimmed)
        rules = z.array(auditRuleSchema).parse(parsed)
      } catch {
        toast.error(t('Custom rules must be a valid JSON array of rule objects'))
        return
      }
      for (const rule of rules) {
        try {
          new RegExp(rule.regex)
        } catch {
          toast.error(t('Invalid regex in rule: {{id}}', { id: rule.id }))
          return
        }
      }
    }
    data.audit.rules = JSON.stringify(rules, null, 2)

    // 校验分组存储策略 JSON
    let groupPolicies: Array<{ group: string; mode: string }> = []
    const trimmedPolicies = data.audit.group_store_prompt_modes.trim()
    if (trimmedPolicies !== '' && trimmedPolicies !== '[]') {
      try {
        const parsed: unknown = JSON.parse(trimmedPolicies)
        groupPolicies = z.array(groupPromptPolicySchema).parse(parsed)
      } catch {
        toast.error(
          t(
            'Group storage policies must be a valid JSON array of {group, mode} objects'
          )
        )
        return
      }
    }
    data.audit.group_store_prompt_modes = JSON.stringify(groupPolicies, null, 2)

    const normalized = normalizeFormValues(data)
    const updates = (
      Object.keys(normalized) as Array<keyof NormalizedAuditValues>
    ).filter((key) => normalized[key] !== baselineRef.current[key])

    if (updates.length === 0) {
      toast.info(t('No changes to save'))
      return
    }

    for (const key of updates) {
      await updateOption.mutateAsync({ key, value: normalized[key] })
    }

    baselineRef.current = normalized
    toast.success(t('Settings saved'))
  }

  const auditEnabled = form.watch('audit.enabled')

  return (
    <SettingsSection title={t('Security Audit')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save audit settings'
          />

          <FormField
            control={form.control}
            name='audit.enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable Security Audit')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Observe-only: record audit events for sensitive content and key sharing signals, without blocking requests'
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

          {auditEnabled && (
            <>
              <FormField
                control={form.control}
                name='audit.prompt_scan_enabled'
                render={({ field }) => (
                  <SettingsSwitchItem>
                    <SettingsSwitchContent>
                      <FormLabel>{t('Scan Prompt Content')}</FormLabel>
                      <FormDescription>
                        {t(
                          'Detect PII and secrets (ID cards, phone numbers, API keys) in outgoing prompts with regex rules'
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

              <FormField
                control={form.control}
                name='audit.store_prompt_mode'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Prompt Storage Mode')}</FormLabel>
                    <Select
                      value={field.value}
                      onValueChange={field.onChange}
                      items={[
                        { value: 'none', label: t('Do not store prompt content') },
                        { value: 'hits', label: t('Store only when a rule hits') },
                        { value: 'all', label: t('Store all prompts (high volume)') },
                      ]}
                    >
                      <FormControl>
                        <SelectTrigger>
                          <SelectValue />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent>
                        <SelectItem value='none'>
                          {t('Do not store prompt content')}
                        </SelectItem>
                        <SelectItem value='hits'>
                          {t('Store only when a rule hits')}
                        </SelectItem>
                        <SelectItem value='all'>
                          {t('Store all prompts (high volume)')}
                        </SelectItem>
                      </SelectContent>
                    </Select>
                    <FormDescription>
                      {t(
                        'Stored prompt content is only visible to admins in audit event details'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <div className='grid gap-x-5 gap-y-6 lg:grid-cols-2'>
                <FormField
                  control={form.control}
                  name='audit.max_scan_bytes'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Max Scan Bytes')}</FormLabel>
                      <FormControl>
                        <Input type='number' {...field} />
                      </FormControl>
                      <FormDescription>
                        {t('Prompts are truncated to this length before scanning')}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='audit.retention_days'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Retention Days')}</FormLabel>
                      <FormControl>
                        <Input type='number' {...field} />
                      </FormControl>
                      <FormDescription>
                        {t('Audit events older than this are deleted daily')}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              <FormField
                control={form.control}
                name='audit.alert_enabled'
                render={({ field }) => (
                  <SettingsSwitchItem>
                    <SettingsSwitchContent>
                      <FormLabel>{t('Alert on Critical Hits')}</FormLabel>
                      <FormDescription>
                        {t(
                          'Push critical-severity events to admins via the configured notify channels (DingTalk / Feishu)'
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

              <div className='border-border mt-2 border-t pt-6'>
                <h4 className='mb-4 text-sm font-medium'>
                  {t('Response Scanning')}
                </h4>
                <FormField
                  control={form.control}
                  name='audit.response_scan_enabled'
                  render={({ field }) => (
                    <SettingsSwitchItem>
                      <SettingsSwitchContent>
                        <FormLabel>{t('Scan Model Responses')}</FormLabel>
                        <FormDescription>
                          {t(
                            'Capture upstream response bytes (including SSE streams) and record an event when malicious-code patterns (reverse shells, pipe-to-shell, credential theft) are detected'
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
                <FormField
                  control={form.control}
                  name='audit.response_max_scan_bytes'
                  render={({ field }) => (
                    <FormItem className='mt-4'>
                      <FormLabel>{t('Response Max Scan Bytes')}</FormLabel>
                      <FormControl>
                        <Input type='number' {...field} />
                      </FormControl>
                      <FormDescription>
                        {t(
                          'Only the first bytes of each response are captured and scanned'
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              <div className='border-border mt-2 border-t pt-6'>
                <h4 className='mb-4 text-sm font-medium'>
                  {t('Key Sharing Detection')}
                </h4>
                <FormField
                  control={form.control}
                  name='audit.key_share_enabled'
                  render={({ field }) => (
                    <SettingsSwitchItem>
                      <SettingsSwitchContent>
                        <FormLabel>{t('Detect Key Sharing')}</FormLabel>
                        <FormDescription>
                          {t(
                            'Track per-token IP / User-Agent diversity and record an event when a token appears to be shared'
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
                <FormField
                  control={form.control}
                  name='audit.geoip_db_path'
                  render={({ field }) => (
                    <FormItem className='mt-4'>
                      <FormLabel>{t('GeoIP Database Path')}</FormLabel>
                      <FormControl>
                        <Input
                          placeholder='/path/to/GeoLite2-City.mmdb'
                          {...field}
                        />
                      </FormControl>
                      <FormDescription>
                        {t(
                          'MaxMind GeoLite2-City mmdb file. Enables impossible-travel detection (critical events when a token is used from physically unreachable locations). Empty disables this detection'
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <div className='mt-4 grid gap-x-5 gap-y-6 lg:grid-cols-3'>
                  <FormField
                    control={form.control}
                    name='audit.key_share_window_minutes'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Window (minutes)')}</FormLabel>
                        <FormControl>
                          <Input type='number' {...field} />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <FormField
                    control={form.control}
                    name='audit.key_share_distinct_ip_threshold'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Distinct IP Threshold')}</FormLabel>
                        <FormControl>
                          <Input type='number' {...field} />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <FormField
                    control={form.control}
                    name='audit.key_share_rapid_ip_threshold'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Rapid IP Threshold')}</FormLabel>
                        <FormControl>
                          <Input type='number' {...field} />
                        </FormControl>
                        <FormDescription>
                          {t('Distinct IPs within the rapid window')}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <FormField
                    control={form.control}
                    name='audit.key_share_rapid_window_minutes'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Rapid Window (minutes)')}</FormLabel>
                        <FormControl>
                          <Input type='number' {...field} />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <FormField
                    control={form.control}
                    name='audit.key_share_suppress_hours'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Suppress (hours)')}</FormLabel>
                        <FormControl>
                          <Input type='number' {...field} />
                        </FormControl>
                        <FormDescription>
                          {t('Suppress duplicate events for the same token')}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>
              </div>

              <div className='border-border mt-2 border-t pt-6'>
                <FormField
                  control={form.control}
                  name='audit.group_store_prompt_modes'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Group Storage Policies (JSON)')}</FormLabel>
                      <FormControl>
                        <Textarea
                          rows={5}
                          className='font-mono text-xs'
                          placeholder='[{"group":"vip","mode":"all"}]'
                          {...field}
                        />
                      </FormControl>
                      <FormDescription>
                        {t(
                          'Per-group prompt storage policy. Priority: user setting > group policy > global mode. Each entry: group, mode (none/hits/all).'
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              <div className='border-border mt-2 border-t pt-6'>
                <h4 className='mb-4 text-sm font-medium'>
                  {t('Prompt Classification (LLM)')}
                </h4>
                <FormField
                  control={form.control}
                  name='audit.classify_enabled'
                  render={({ field }) => (
                    <SettingsSwitchItem>
                      <SettingsSwitchContent>
                        <FormLabel>{t('Classify Stored Prompts')}</FormLabel>
                        <FormDescription>
                          {t(
                            'A scheduled task sends stored prompts to the configured channel for category / skill-title classification. Classification failures are logged and silently skipped; the relay path is never affected'
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
                <div className='mt-4 grid gap-x-5 gap-y-6 lg:grid-cols-2'>
                  <FormField
                    control={form.control}
                    name='audit.classify_channel_id'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Classification Channel ID')}</FormLabel>
                        <FormControl>
                          <Input type='number' {...field} />
                        </FormControl>
                        <FormDescription>
                          {t(
                            'An OpenAI-compatible channel used for classification. 0 disables the task'
                          )}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <FormField
                    control={form.control}
                    name='audit.classify_model'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Classification Model')}</FormLabel>
                        <FormControl>
                          <Input placeholder='gpt-4o-mini' {...field} />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <FormField
                    control={form.control}
                    name='audit.classify_interval_minutes'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Interval (minutes)')}</FormLabel>
                        <FormControl>
                          <Input type='number' {...field} />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <FormField
                    control={form.control}
                    name='audit.classify_batch_size'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Batch Size')}</FormLabel>
                        <FormControl>
                          <Input type='number' {...field} />
                        </FormControl>
                        <FormDescription>
                          {t('Prompts classified per task run')}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>
              </div>

              <div className='border-border mt-2 border-t pt-6'>
                <FormField
                  control={form.control}
                  name='audit.rules'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Custom Rules (JSON)')}</FormLabel>
                      <FormControl>
                        <Textarea
                          rows={8}
                          className='font-mono text-xs'
                          placeholder='[{"id":"custom.internal_domain","name":"Internal domain","regex":"internal\\.example\\.com","severity":"warning","enabled":true}]'
                          {...field}
                        />
                      </FormControl>
                      <FormDescription>
                        {t(
                          'Custom regex rules merged with the built-in PII rules. Each rule: id (stable), name, regex, severity (info/warning/critical), enabled.'
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
            </>
          )}
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
