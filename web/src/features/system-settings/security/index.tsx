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
import { SettingsPage } from '../components/settings-page'
import type { SecuritySettings } from '../types'
import {
  SECURITY_DEFAULT_SECTION,
  getSecuritySectionContent,
  getSecuritySectionMeta,
} from './section-registry.tsx'

const defaultSecuritySettings: SecuritySettings = {
  ModelRequestRateLimitEnabled: false,
  ModelRequestRateLimitCount: 0,
  ModelRequestRateLimitSuccessCount: 1000,
  ModelRequestRateLimitDurationMinutes: 1,
  ModelRequestRateLimitGroup: '',
  CheckSensitiveEnabled: false,
  CheckSensitiveOnPromptEnabled: false,
  SensitiveWords: '',
  'fetch_setting.enable_ssrf_protection': true,
  'fetch_setting.allow_private_ip': false,
  'fetch_setting.domain_filter_mode': false,
  'fetch_setting.ip_filter_mode': false,
  'fetch_setting.domain_list': [],
  'fetch_setting.ip_list': [],
  'fetch_setting.allowed_ports': [],
  'fetch_setting.apply_ip_filter_for_domain': false,
  'token_setting.max_user_tokens': 1000,
  'audit.enabled': false,
  'audit.prompt_scan_enabled': true,
  'audit.store_prompt_mode': 'none',
  'audit.max_scan_bytes': 32768,
  'audit.alert_enabled': true,
  'audit.retention_days': 90,
  'audit.key_share_enabled': true,
  'audit.key_share_window_minutes': 1440,
  'audit.key_share_distinct_ip_threshold': 5,
  'audit.key_share_rapid_window_minutes': 10,
  'audit.key_share_rapid_ip_threshold': 3,
  'audit.key_share_suppress_hours': 24,
  'audit.response_scan_enabled': true,
  'audit.response_max_scan_bytes': 65536,
  'audit.rules': [],
  'audit.group_store_prompt_modes': [],
  'audit.geoip_db_path': '',
  'audit.classify_enabled': false,
  'audit.classify_channel_id': 0,
  'audit.classify_model': '',
  'audit.classify_interval_minutes': 60,
  'audit.classify_batch_size': 20,
}

export function SecuritySettings() {
  return (
    <SettingsPage
      routePath='/_authenticated/system-settings/security/$section'
      defaultSettings={defaultSecuritySettings}
      defaultSection={SECURITY_DEFAULT_SECTION}
      getSectionContent={getSecuritySectionContent}
      getSectionMeta={getSecuritySectionMeta}
    />
  )
}
