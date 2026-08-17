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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  buildScheduleJSON,
  CHANNEL_FORM_DEFAULT_VALUES,
  transformChannelToFormDefaults,
  transformFormDataToUpdatePayload,
  transformFormDataToCreatePayload,
} from '../channel-form'
import type { Channel } from '../../types'

const BASE_CHANNEL = {
  id: 1,
  type: 1,
  key: '',
  status: 1,
  name: 'test-channel',
  created_time: 0,
  test_time: 0,
  response_time: 0,
  balance: 0,
  balance_updated_time: 0,
  used_quota: 0,
  models: 'gpt-4',
  group: 'default',
  other: '',
  other_info: '',
  channel_info: {
    is_multi_key: false,
    multi_key_size: 0,
    multi_key_polling_index: 0,
    multi_key_mode: 'random',
  },
  settings: '{}',
  remark: '',
  max_input_tokens: 0,
} as unknown as Channel

describe('buildScheduleJSON', () => {
  test('returns null when schedule is disabled', () => {
    const formData = {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      schedule_enabled: false,
      schedule_windows: [{ days: [1, 2, 3, 4, 5], start: '00:30', end: '08:30' }],
    }
    assert.equal(buildScheduleJSON(formData), null)
  })

  test('returns null when windows array is empty', () => {
    const formData = {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      schedule_enabled: true,
      schedule_windows: [],
    }
    assert.equal(buildScheduleJSON(formData), null)
  })

  test('returns correct JSON when enabled with windows', () => {
    const formData = {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      schedule_enabled: true,
      schedule_timezone: 'Asia/Shanghai',
      schedule_windows: [
        { days: [1, 2, 3, 4, 5], start: '00:30', end: '08:30' },
        { days: [0, 6], start: '22:00', end: '02:00' },
      ],
    }
    const result = buildScheduleJSON(formData)
    assert.ok(result !== null)
    const parsed = JSON.parse(result as string)
    assert.equal(parsed.enabled, true)
    assert.equal(parsed.timezone, 'Asia/Shanghai')
    assert.equal(parsed.windows.length, 2)
    assert.deepEqual(parsed.windows[0], {
      days: [1, 2, 3, 4, 5],
      start: '00:30',
      end: '08:30',
    })
  })
})

describe('transformChannelToFormDefaults schedule parsing', () => {
  test('parses valid schedule JSON into form fields', () => {
    const schedule = JSON.stringify({
      enabled: true,
      timezone: 'Asia/Shanghai',
      windows: [{ days: [1, 2, 3, 4, 5], start: '00:30', end: '08:30' }],
    })
    const channel = { ...BASE_CHANNEL, schedule }
    const form = transformChannelToFormDefaults(channel)
    assert.equal(form.schedule_enabled, true)
    assert.equal(form.schedule_timezone, 'Asia/Shanghai')
    assert.deepEqual(form.schedule_windows, [
      { days: [1, 2, 3, 4, 5], start: '00:30', end: '08:30' },
    ])
  })

  test('leaves defaults when schedule is null', () => {
    const channel = { ...BASE_CHANNEL, schedule: null }
    const form = transformChannelToFormDefaults(channel)
    assert.equal(form.schedule_enabled, false)
    assert.equal(form.schedule_windows?.length, 0)
  })

  test('leaves defaults when schedule JSON is invalid', () => {
    const channel = { ...BASE_CHANNEL, schedule: '{invalid json' }
    const form = transformChannelToFormDefaults(channel)
    assert.equal(form.schedule_enabled, false)
    assert.equal(form.schedule_windows?.length, 0)
  })

  test('round-trip: parse then build returns equivalent JSON', () => {
    const schedule = JSON.stringify({
      enabled: true,
      timezone: 'UTC',
      windows: [{ days: [0, 6], start: '22:00', end: '02:00' }],
    })
    const channel = { ...BASE_CHANNEL, schedule }
    const form = transformChannelToFormDefaults(channel)
    const rebuilt = buildScheduleJSON(form)
    assert.ok(rebuilt !== null)
    const parsed = JSON.parse(rebuilt as string)
    assert.equal(parsed.enabled, true)
    assert.equal(parsed.timezone, 'UTC')
    assert.deepEqual(parsed.windows, [
      { days: [0, 6], start: '22:00', end: '02:00' },
    ])
  })
})

describe('payload transforms include schedule', () => {
  test('create payload includes schedule JSON when enabled', () => {
    const formData = {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      schedule_enabled: true,
      schedule_timezone: 'Asia/Shanghai',
      schedule_windows: [{ days: [1, 2, 3, 4, 5], start: '00:30', end: '08:30' }],
    }
    const { channel } = transformFormDataToCreatePayload(formData)
    assert.ok(channel.schedule !== null && channel.schedule !== undefined)
    const parsed = JSON.parse(channel.schedule as string)
    assert.equal(parsed.enabled, true)
  })

  test('create payload sets schedule to null when disabled', () => {
    const formData = {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      schedule_enabled: false,
      schedule_windows: [],
    }
    const { channel } = transformFormDataToCreatePayload(formData)
    assert.equal(channel.schedule, null)
  })

  test('update payload sends explicit empty string when schedule disabled', () => {
    const formData = {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      schedule_enabled: false,
      schedule_windows: [],
    }
    const payload = transformFormDataToUpdatePayload(formData, 42)
    assert.equal(payload.schedule, '')
  })

  test('update payload includes schedule JSON when enabled', () => {
    const formData = {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      schedule_enabled: true,
      schedule_timezone: 'UTC',
      schedule_windows: [{ days: [1, 2, 3, 4, 5], start: '00:30', end: '08:30' }],
    }
    const payload = transformFormDataToUpdatePayload(formData, 42)
    assert.ok(payload.schedule)
    const parsed = JSON.parse(payload.schedule as string)
    assert.equal(parsed.enabled, true)
    assert.equal(parsed.timezone, 'UTC')
  })
})
