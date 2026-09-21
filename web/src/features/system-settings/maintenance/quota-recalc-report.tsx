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
import { useTranslation } from 'react-i18next'

import type { QuotaRecalcReport } from './quota-recalc-api'

const SKIP_REASON_KEYS: Record<string, string> = {
  unparsable_log_other: 'Log usage metadata could not be parsed',
  unsupported_billing_mode: 'Billing mode cannot be reproduced',
  expression_needs_request_probe: 'Expression depends on the original request',
  no_usage_recorded: 'No token usage recorded',
  not_a_consume_log: 'Not a consumption log',
}

function ReportRow({ label, value }: { label: string; value: string }) {
  return (
    <div className='flex items-center justify-between gap-4 text-sm'>
      <span className='text-muted-foreground'>{label}</span>
      <span className='font-medium tabular-nums'>{value}</span>
    </div>
  )
}

export function QuotaRecalcReportView({
  report,
}: {
  report: QuotaRecalcReport
}) {
  const { t } = useTranslation()
  const delta = report.new_quota_sum - report.old_quota_sum
  const formattedDelta = `${delta > 0 ? '+' : ''}${delta}`

  return (
    <div className='space-y-3'>
      <div className='space-y-2 rounded-lg border p-3'>
        <ReportRow
          label={t('Scanned logs')}
          value={String(report.scanned_logs)}
        />
        <ReportRow
          label={t('Recomputable logs')}
          value={String(report.recomputable_logs)}
        />
        <ReportRow
          label={t('Logs with changed quota')}
          value={String(report.changed_logs)}
        />
        {report.applied && (
          <ReportRow
            label={t('Updated logs')}
            value={String(report.updated_logs)}
          />
        )}
        {report.applied && (
          <ReportRow
            label={t('Updated dashboard rows')}
            value={String(report.updated_buckets)}
          />
        )}
      </div>

      <div className='space-y-2 rounded-lg border p-3'>
        <ReportRow
          label={t('Total quota before')}
          value={String(report.old_quota_sum)}
        />
        <ReportRow
          label={t('Total quota after')}
          value={String(report.new_quota_sum)}
        />
        <ReportRow label={t('Difference')} value={formattedDelta} />
      </div>

      {report.skipped_logs > 0 && (
        <div className='space-y-2 rounded-lg border p-3'>
          <p className='text-sm font-medium'>
            {t('Skipped logs: {{count}}', { count: report.skipped_logs })}
          </p>
          <ul className='text-muted-foreground space-y-1 text-sm'>
            {Object.entries(report.skip_reasons ?? {}).map(([reason, count]) => (
              <li key={reason} className='flex justify-between gap-4'>
                <span>
                  {t(SKIP_REASON_KEYS[reason] ?? 'Unsupported pricing')}
                </span>
                <span className='tabular-nums'>{count}</span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}
