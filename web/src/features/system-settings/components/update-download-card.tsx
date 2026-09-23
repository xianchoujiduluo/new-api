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
import { CheckCircle2, Download, Loader2, TriangleAlert } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Progress } from '@/components/ui/progress'

import type { UpdateDownloadProgress } from '../types'

const TRANSFER_STATES = new Set<string>([
  'fetching',
  'downloading',
  'verifying',
  'installing',
  'extracting',
])

/** Byte transfer is the only measurable phase; the rest report as stages. */
function isMeasurable(job: UpdateDownloadProgress): boolean {
  return job.state === 'downloading' && job.bytes_total > 0
}

function formatBytes(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return '0 MB'
  return `${(value / 1024 / 1024).toFixed(1)} MB`
}

type UpdateDownloadCardProps = {
  job?: UpdateDownloadProgress
  /** Whether a release is staged on disk; survives page reloads and restarts. */
  staged: boolean
  /** Ready-state headline, already translated and including the version. */
  readyTitle: string
  readyHint: string
  activateLabel: string
  discardLabel: string
  /** Shown while fetching or downloading, already translated. */
  progressTitle: string
  onActivate: () => void
  onDiscard: () => void
  discarding: boolean
  /** Present only when activation waits for the process to come back. */
  activating?: { label: string; seconds: number }
}

export function UpdateDownloadCard(props: UpdateDownloadCardProps) {
  const { t } = useTranslation()
  const { job } = props

  if (props.activating) {
    return (
      <Shell tone='active'>
        <Loader2 className='mt-0.5 h-4 w-4 shrink-0 animate-spin' />
        <div className='space-y-1'>
          <p className='text-sm font-medium'>{props.activating.label}</p>
          <p className='text-muted-foreground text-sm'>
            {t(
              'This page reconnects automatically once the new version is up.'
            )}{' '}
            {t('Waiting: {{seconds}}s', { seconds: props.activating.seconds })}
          </p>
        </div>
      </Shell>
    )
  }

  if (job?.state === 'failed') {
    return (
      <Shell tone='danger'>
        <TriangleAlert className='mt-0.5 h-4 w-4 shrink-0' />
        <div className='min-w-0 space-y-1'>
          <p className='text-sm font-medium'>{t('Download failed')}</p>
          <p className='text-muted-foreground break-words text-sm'>
            {job.error || t('Unknown error')}
          </p>
        </div>
      </Shell>
    )
  }

  if (job && TRANSFER_STATES.has(job.state)) {
    return (
      <Shell tone='active'>
        <Download className='mt-0.5 h-4 w-4 shrink-0' />
        <div className='min-w-0 flex-1 space-y-2'>
          <p className='text-sm font-medium'>{props.progressTitle}</p>
          {isMeasurable(job) ? (
            <>
              <Progress value={job.percent} />
              <p className='text-muted-foreground text-xs tabular-nums'>
                {`${job.percent.toFixed(0)}% · ${formatBytes(
                  job.bytes_done
                )} / ${formatBytes(job.bytes_total)}`}
              </p>
            </>
          ) : (
            <p className='text-muted-foreground text-sm'>
              {job.message || t('Working…')}
            </p>
          )}
        </div>
      </Shell>
    )
  }

  if (!props.staged) return null

  return (
    <Shell tone='success'>
      <CheckCircle2 className='mt-0.5 h-4 w-4 shrink-0' />
      <div className='flex-1 space-y-3'>
        <div className='space-y-1'>
          <p className='text-sm font-medium'>{props.readyTitle}</p>
          <p className='text-muted-foreground text-sm'>{props.readyHint}</p>
        </div>
        <div className='flex flex-wrap gap-2'>
          <Button type='button' size='sm' onClick={props.onActivate}>
            {props.activateLabel}
          </Button>
          <Button
            type='button'
            size='sm'
            variant='outline'
            onClick={props.onDiscard}
            disabled={props.discarding}
          >
            {props.discardLabel}
          </Button>
        </div>
      </div>
    </Shell>
  )
}

function Shell({
  tone,
  children,
}: {
  tone: 'active' | 'success' | 'danger'
  children: React.ReactNode
}) {
  const tones = {
    active: 'border-border bg-muted/40',
    success: 'border-emerald-500/40 bg-emerald-500/5',
    danger: 'border-destructive/40 bg-destructive/5',
  }
  const text = {
    active: 'text-muted-foreground',
    success: 'text-emerald-600 dark:text-emerald-400',
    danger: 'text-destructive',
  }
  return (
    <div
      className={`flex gap-2 rounded-lg border p-3 ${tones[tone]} ${text[tone]}`}
    >
      {children}
    </div>
  )
}
