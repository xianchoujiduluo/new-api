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
import { CalculatorIcon, RefreshCwIcon } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { getUserModels } from '@/lib/api'
import { handleServerError } from '@/lib/handle-server-error'

import { CompactDateTimeRangePicker } from '@/features/usage-logs/components/compact-date-time-range-picker'
import { SettingsSection } from '../components/settings-section'
import { QuotaRecalcReportView } from './quota-recalc-report'
import {
  recalcLogQuota,
  type QuotaRecalcReport,
} from './quota-recalc-api'

export function LogQuotaRecalcSection() {
  const { t } = useTranslation()
  const [modelName, setModelName] = useState('')
  const [range, setRange] = useState<{ start?: Date; end?: Date }>({})
  const [models, setModels] = useState<string[]>([])
  const [running, setRunning] = useState(false)
  const [report, setReport] = useState<QuotaRecalcReport | null>(null)
  const [reportOpen, setReportOpen] = useState(false)
  const [confirmOpen, setConfirmOpen] = useState(false)

  useEffect(() => {
    getUserModels()
      .then((res) => setModels(res.data ?? []))
      .catch(() => setModels([]))
  }, [])

  const payload = () => ({
    model_name: modelName.trim(),
    start_timestamp: range.start
      ? Math.floor(range.start.getTime() / 1000)
      : 0,
    end_timestamp: range.end ? Math.floor(range.end.getTime() / 1000) : 0,
  })

  const run = async (apply: boolean) => {
    if (!modelName.trim() || !range.start || !range.end) {
      toast.error(t('Select a model and a time range first'))
      return
    }
    setRunning(true)
    try {
      const result = await recalcLogQuota({ ...payload(), apply })
      setReport(result)
      setReportOpen(true)
      if (apply) {
        toast.success(
          t('Updated {{count}} logs', { count: result.updated_logs })
        )
      }
    } catch (error) {
      handleServerError(error)
    } finally {
      setRunning(false)
    }
  }

  const recomputable = report?.recomputable_logs ?? 0

  return (
    <SettingsSection
      title={t('Quota Recalculation')}
      titleProps={{ id: 'quota-recalculation' }}
    >
      <div className='space-y-3'>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Re-prices historical consumption logs with the current billing settings. Peak and off-peak tiers are evaluated at the original timestamp of each log. Logs that cannot be reproduced are skipped and listed.'
          )}
        </p>

        <div className='flex flex-wrap items-center gap-2'>
          <Input
            className='w-64'
            list='quota-recalc-models'
            placeholder={t('Model Name')}
            value={modelName}
            onChange={(event) => setModelName(event.target.value)}
          />
          <datalist id='quota-recalc-models'>
            {models.map((name) => (
              <option key={name} value={name} />
            ))}
          </datalist>

          <CompactDateTimeRangePicker start={range.start} end={range.end} onChange={setRange} />

          <Button
            type='button'
            variant='secondary'
            disabled={running}
            onClick={() => run(false)}
          >
            <CalculatorIcon className='me-2 h-4 w-4' />
            {t('Preview')}
          </Button>
          <Button
            type='button'
            variant='destructive'
            disabled={running}
            onClick={() => setConfirmOpen(true)}
          >
            <RefreshCwIcon className='me-2 h-4 w-4' />
            {t('Apply')}
          </Button>
        </div>
      </div>

      <Dialog
        open={reportOpen}
        onOpenChange={setReportOpen}
        title={t('Quota Recalculation Result')}
        description={report?.applied ? t('Changes applied') : t('Preview only')}
        contentHeight='auto'
        contentClassName='max-h-[80vh] overflow-y-auto'
        bodyClassName='space-y-4'
        footer={
          <Button type='button' onClick={() => setReportOpen(false)}>
            {t('Close')}
          </Button>
        }
      >
        {report && <QuotaRecalcReportView report={report} />}
      </Dialog>

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t('Apply quota recalculation?')}
        desc={
          report?.applied
            ? t('This will overwrite the quota of the selected logs.')
            : t(
                'Run a preview first. Applying overwrites the quota of the selected logs and cannot be undone.'
              )
        }
        destructive
        confirmText={t('Apply')}
        isLoading={running}
        handleConfirm={() => {
          setConfirmOpen(false)
          void run(true)
        }}
      >
        {recomputable > 0 && (
          <p className='text-sm'>
            {t('Last preview matched {{count}} logs.', { count: recomputable })}
          </p>
        )}
      </ConfirmDialog>
    </SettingsSection>
  )
}
