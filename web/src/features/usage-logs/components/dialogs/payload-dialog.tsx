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
import { useQuery } from '@tanstack/react-query'
import { AlertCircle } from 'lucide-react'
import type React from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Dialog } from '@/components/dialog'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'

import { getRequestPayload } from '../../api'
import type { RequestPayload } from '../../types'

interface PayloadDialogProps {
  requestId: string
  isAdmin: boolean
  open: boolean
  onOpenChange: (open: boolean) => void
}

function prettyJson(value: string): string {
  const trimmed = value.trim()
  if (!trimmed) return ''
  try {
    return JSON.stringify(JSON.parse(trimmed), null, 2)
  } catch {
    return value
  }
}

function PayloadSection(props: {
  title: string
  content: string
  truncated?: boolean
}) {
  const { t } = useTranslation()
  return (
    <section className='space-y-2'>
      <div className='flex items-center justify-between gap-2'>
        <span className='text-sm font-semibold'>{props.title}</span>
        <div className='flex items-center gap-1.5'>
          {props.truncated && <Badge variant='outline'>{t('Truncated')}</Badge>}
          <CopyButton
            value={props.content}
            aria-label={`${t('Copy to clipboard')}`}
          />
        </div>
      </div>
      <pre className='bg-muted/50 max-h-80 overflow-auto rounded-md border p-3 text-xs break-all whitespace-pre-wrap'>
        {props.content || '-'}
      </pre>
    </section>
  )
}

export function PayloadDialog(props: PayloadDialogProps) {
  const { t } = useTranslation()
  const payloadQuery = useQuery({
    queryKey: ['usage-logs', 'request-payload', props.requestId],
    queryFn: () => getRequestPayload(props.requestId, props.isAdmin),
    enabled: props.open && !!props.requestId,
    retry: false,
    staleTime: 5 * 60_000,
  })

  const renderBody = (payload: RequestPayload) => (
    <div className='space-y-4'>
      <PayloadSection
        title={t('Request Headers')}
        content={prettyJson(payload.request_headers)}
      />
      <PayloadSection
        title={t('Request Body')}
        content={prettyJson(payload.request_body)}
        truncated={payload.is_truncated}
      />
      <PayloadSection
        title={t('Response Headers')}
        content={prettyJson(payload.response_headers)}
      />
      <PayloadSection
        title={t('Response Body')}
        content={prettyJson(payload.response_body)}
        truncated={payload.is_truncated}
      />
    </div>
  )

  let body: React.ReactNode = null
  if (payloadQuery.isPending) {
    body = (
      <div className='space-y-3' aria-label={t('Loading...')}>
        <Skeleton className='h-24 w-full' />
        <Skeleton className='h-24 w-full' />
      </div>
    )
  } else if (payloadQuery.isError) {
    body = (
      <Alert variant='destructive'>
        <AlertCircle aria-hidden='true' />
        <AlertTitle>{t('Failed to load request details')}</AlertTitle>
        <AlertDescription>{t('Payload unavailable')}</AlertDescription>
      </Alert>
    )
  } else if (payloadQuery.data) {
    body = renderBody(payloadQuery.data)
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Request Details')}
      description={t('Raw request and response captured for this call')}
      contentClassName='sm:max-w-3xl'
      contentHeight='auto'
    >
      {body}
    </Dialog>
  )
}
