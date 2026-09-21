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
import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

export type QuotaRecalcReport = {
  model_name: string
  start_timestamp: number
  end_timestamp: number
  applied: boolean
  scanned_logs: number
  recomputable_logs: number
  skipped_logs: number
  changed_logs: number
  updated_logs: number
  old_quota_sum: number
  new_quota_sum: number
  skip_reasons: Record<string, number> | null
  affected_buckets: number
  updated_buckets: number
}

export type QuotaRecalcPayload = {
  model_name: string
  start_timestamp: number
  end_timestamp: number
  apply: boolean
}

/**
 * Re-price historical consume logs against the current billing settings.
 * `apply: false` performs a pure dry run that reports the deltas without
 * writing anything.
 */
export async function recalcLogQuota(
  payload: QuotaRecalcPayload
): Promise<QuotaRecalcReport> {
  const res = await api.post('/api/log/quota_recalc', payload)
  const body = requireServerSuccess(
    res.data as { success: boolean; message?: string; data: QuotaRecalcReport }
  )
  return body.data
}
