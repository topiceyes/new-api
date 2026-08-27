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
import { RateLimitSection } from '../request-limits/rate-limit-section'
import { SensitiveWordsSection } from '../request-limits/sensitive-words-section'
import { SSRFSection } from '../request-limits/ssrf-section'
import { TokenLimitSection } from '../request-limits/token-limit-section'
import type { SecuritySettings } from '../types'
import { createSectionRegistry } from '../utils/section-registry'
import { AuditSection } from './audit-section'

const SECURITY_SECTIONS = [
  {
    id: 'rate-limit',
    titleKey: 'Rate Limiting',
    build: (settings: SecuritySettings) => (
      <RateLimitSection
        defaultValues={{
          ModelRequestRateLimitEnabled: settings.ModelRequestRateLimitEnabled,
          ModelRequestRateLimitCount: settings.ModelRequestRateLimitCount,
          ModelRequestRateLimitSuccessCount:
            settings.ModelRequestRateLimitSuccessCount,
          ModelRequestRateLimitDurationMinutes:
            settings.ModelRequestRateLimitDurationMinutes,
          ModelRequestRateLimitGroup: settings.ModelRequestRateLimitGroup,
        }}
      />
    ),
  },
  {
    id: 'sensitive-words',
    titleKey: 'Sensitive Words',
    build: (settings: SecuritySettings) => (
      <SensitiveWordsSection
        defaultValues={{
          CheckSensitiveEnabled: settings.CheckSensitiveEnabled,
          CheckSensitiveOnPromptEnabled: settings.CheckSensitiveOnPromptEnabled,
          SensitiveWords: settings.SensitiveWords,
        }}
      />
    ),
  },
  {
    id: 'ssrf',
    titleKey: 'SSRF Protection',
    build: (settings: SecuritySettings) => (
      <SSRFSection
        defaultValues={{
          'fetch_setting.enable_ssrf_protection':
            settings['fetch_setting.enable_ssrf_protection'],
          'fetch_setting.allow_private_ip':
            settings['fetch_setting.allow_private_ip'],
          'fetch_setting.domain_filter_mode':
            settings['fetch_setting.domain_filter_mode'],
          'fetch_setting.ip_filter_mode':
            settings['fetch_setting.ip_filter_mode'],
          'fetch_setting.domain_list': settings['fetch_setting.domain_list'],
          'fetch_setting.ip_list': settings['fetch_setting.ip_list'],
          'fetch_setting.allowed_ports':
            settings['fetch_setting.allowed_ports'],
          'fetch_setting.apply_ip_filter_for_domain':
            settings['fetch_setting.apply_ip_filter_for_domain'],
        }}
      />
    ),
  },
  {
    id: 'audit',
    titleKey: 'Security Audit',
    build: (settings: SecuritySettings) => (
      <AuditSection
        defaultValues={{
          'audit.enabled': settings['audit.enabled'],
          'audit.prompt_scan_enabled': settings['audit.prompt_scan_enabled'],
          'audit.store_prompt_mode': settings['audit.store_prompt_mode'],
          'audit.max_scan_bytes': settings['audit.max_scan_bytes'],
          'audit.alert_enabled': settings['audit.alert_enabled'],
          'audit.retention_days': settings['audit.retention_days'],
          'audit.key_share_enabled': settings['audit.key_share_enabled'],
          'audit.key_share_window_minutes':
            settings['audit.key_share_window_minutes'],
          'audit.key_share_distinct_ip_threshold':
            settings['audit.key_share_distinct_ip_threshold'],
          'audit.key_share_rapid_window_minutes':
            settings['audit.key_share_rapid_window_minutes'],
          'audit.key_share_rapid_ip_threshold':
            settings['audit.key_share_rapid_ip_threshold'],
          'audit.key_share_suppress_hours':
            settings['audit.key_share_suppress_hours'],
          'audit.response_scan_enabled':
            settings['audit.response_scan_enabled'],
          'audit.response_max_scan_bytes':
            settings['audit.response_max_scan_bytes'],
          'audit.geoip_db_path': settings['audit.geoip_db_path'],
          'audit.group_store_prompt_modes':
            settings['audit.group_store_prompt_modes'],
          'audit.classify_enabled': settings['audit.classify_enabled'],
          'audit.classify_channel_id': settings['audit.classify_channel_id'],
          'audit.classify_model': settings['audit.classify_model'],
          'audit.classify_interval_minutes':
            settings['audit.classify_interval_minutes'],
          'audit.classify_batch_size': settings['audit.classify_batch_size'],
          'audit.rules': settings['audit.rules'],
        }}
      />
    ),
  },
  {
    id: 'token-limits',
    titleKey: 'Token Limits',
    build: (settings: SecuritySettings) => (
      <TokenLimitSection
        defaultValues={{
          'token_setting.max_user_tokens':
            settings['token_setting.max_user_tokens'],
        }}
      />
    ),
  },
] as const

export type SecuritySectionId = (typeof SECURITY_SECTIONS)[number]['id']

const securityRegistry = createSectionRegistry<
  SecuritySectionId,
  SecuritySettings
>({
  sections: SECURITY_SECTIONS,
  defaultSection: 'rate-limit',
  basePath: '/system-settings/security',
  urlStyle: 'path',
})

export const SECURITY_SECTION_IDS = securityRegistry.sectionIds
export const SECURITY_DEFAULT_SECTION = securityRegistry.defaultSection
export const getSecuritySectionNavItems = securityRegistry.getSectionNavItems
export const getSecuritySectionContent = securityRegistry.getSectionContent
export const getSecuritySectionMeta = securityRegistry.getSectionMeta
