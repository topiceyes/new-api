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
import { describe, expect, it } from 'vitest'

import { formatDurationSeconds } from './format'

describe('formatDurationSeconds', () => {
  it('renders missing data as dash', () => {
    expect(formatDurationSeconds(0)).toBe('-')
    expect(formatDurationSeconds(-5)).toBe('-')
    expect(formatDurationSeconds(Number.NaN)).toBe('-')
    expect(formatDurationSeconds(Infinity)).toBe('-')
  })

  it('renders sub-minute durations in seconds with one decimal', () => {
    expect(formatDurationSeconds(1.24)).toBe('1.2s')
    expect(formatDurationSeconds(4)).toBe('4s')
    expect(formatDurationSeconds(59.96)).toBe('60s')
  })

  it('renders minute-scale durations as Xm Ys', () => {
    expect(formatDurationSeconds(185)).toBe('3m 5s')
    expect(formatDurationSeconds(60)).toBe('1m 0s')
    expect(formatDurationSeconds(3599)).toBe('59m 59s')
  })

  it('renders hour-scale durations as Xh Ym', () => {
    expect(formatDurationSeconds(3661)).toBe('1h 1m')
    expect(formatDurationSeconds(7200)).toBe('2h 0m')
  })
})
