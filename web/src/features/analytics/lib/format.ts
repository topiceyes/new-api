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

// Average request duration in seconds → compact human form.
// Non-positive/NaN input means "no data" and renders as '-'.
export function formatDurationSeconds(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) {
    return '-'
  }
  if (seconds < 60) {
    return `${Number(seconds.toFixed(1))}s`
  }
  const totalSeconds = Math.round(seconds)
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const secs = totalSeconds % 60
  if (hours > 0) {
    return `${hours}h ${minutes}m`
  }
  return `${minutes}m ${secs}s`
}
