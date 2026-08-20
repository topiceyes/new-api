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
  formatPeriodEnd,
  periodLabelKey,
  providerLabel,
  usageLevel,
} from '../index'

describe('usageLevel thresholds', () => {
  test('below 80 is green', () => {
    assert.equal(usageLevel(0), 'green')
    assert.equal(usageLevel(79), 'green')
    assert.equal(usageLevel(79.9), 'green')
  })
  test('80 to below 90 is yellow', () => {
    assert.equal(usageLevel(80), 'yellow')
    assert.equal(usageLevel(85), 'yellow')
    assert.equal(usageLevel(89.9), 'yellow')
  })
  test('90 and above is red', () => {
    assert.equal(usageLevel(90), 'red')
    assert.equal(usageLevel(95), 'red')
    assert.equal(usageLevel(100), 'red')
  })
})

describe('periodLabelKey', () => {
  test('maps known periods', () => {
    assert.equal(periodLabelKey('5h'), 'Every 5 hours')
    assert.equal(periodLabelKey('weekly'), 'Weekly')
    assert.equal(periodLabelKey('monthly'), 'Monthly')
  })
  test('falls back to raw period for unknown', () => {
    assert.equal(periodLabelKey('daily'), 'daily')
  })
})

describe('formatPeriodEnd', () => {
  test('returns dash for empty/invalid', () => {
    assert.equal(formatPeriodEnd(0), '-')
    assert.equal(formatPeriodEnd(-5), '-')
  })
  test('formats a unix-second timestamp', () => {
    // 1970-01-01T00:00:00Z in local time should still produce a valid string
    const out = formatPeriodEnd(1760000000)
    assert.match(out, /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}$/)
  })
})

describe('providerLabel', () => {
  test('maps known providers', () => {
    assert.equal(providerLabel('minimax'), 'MiniMax')
    assert.equal(providerLabel('kimi'), 'Kimi')
  })
  test('falls back to raw provider for unknown', () => {
    assert.equal(providerLabel('acme'), 'acme')
  })
})
