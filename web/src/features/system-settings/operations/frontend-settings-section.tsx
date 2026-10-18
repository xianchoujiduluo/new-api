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
import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Download, RefreshCw, RotateCcw } from 'lucide-react'
import { useCallback } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Form } from '@/components/ui/form'

import {
  getFrontendDownloadJob,
  getFrontendUpdateStatus,
} from '../api'
import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { UpdateDownloadCard } from '../components/update-download-card'
import { VersionValue } from './backend-settings-status'
import { useResetForm } from '../hooks/use-reset-form'
import {
  FrontendArchiveUrlField,
  FrontendDownloadProxyField,
  FrontendManifestUrlField,
  type FrontendFormValues,
} from './frontend-settings-fields'
import { createFrontendSchema } from './frontend-settings-schema'
import { useFrontendUpdateActions } from './use-frontend-update-actions'

const TRANSFER_STATES = new Set<string>([
  'fetching',
  'downloading',
  'verifying',
  'installing',
  'extracting',
])

type FrontendSettingsSectionProps = {
  defaultDownloadUrl: string
  defaultDownloadProxy: string
  defaultManifestUrl: string
}

export function FrontendSettingsSection(props: FrontendSettingsSectionProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const defaults = {
    downloadUrl: props.defaultDownloadUrl ?? '',
    downloadProxy: props.defaultDownloadProxy ?? '',
    manifestUrl: props.defaultManifestUrl ?? '',
  }
  const form = useForm<FrontendFormValues>({
    resolver: zodResolver(createFrontendSchema(t)),
    defaultValues: defaults,
  })
  useResetForm(form, defaults)

  const refresh = useCallback(async () => {
    await queryClient.invalidateQueries({ queryKey: ['frontend-update-status'] })
    await queryClient.invalidateQueries({ queryKey: ['frontend-download-job'] })
  }, [queryClient])

  const statusQuery = useQuery({
    queryKey: ['frontend-update-status'],
    queryFn: getFrontendUpdateStatus,
    refetchInterval: 30_000,
  })
  const status = statusQuery.data?.success ? statusQuery.data.data : undefined
  const stagedVersion = status?.pending_version || undefined

  // Poll only while a transfer is running; an idle page stays quiet.
  const jobQuery = useQuery({
    queryKey: ['frontend-download-job'],
    queryFn: getFrontendDownloadJob,
    refetchInterval: (query) => {
      const state = query.state.data?.data?.state
      if (!state) return false
      return TRANSFER_STATES.has(state) ? 1_000 : false
    },
  })
  const job = jobQuery.data?.success ? jobQuery.data.data : undefined
  const staged = job?.state === 'ready'
  const transferActive = TRANSFER_STATES.has(job?.state ?? '')

  const actions = useFrontendUpdateActions({ form, defaults, refresh })

  return (
    <SettingsSection title={t('Frontend')}>
      <Form {...form}>
        <SettingsForm
          onSubmit={form.handleSubmit(actions.handleSave)}
          autoComplete='off'
        >
          <SettingsPageFormActions
            onSave={form.handleSubmit(actions.handleSave)}
            isSaving={actions.savingSettings || actions.starting}
            saveLabel='Save Frontend settings'
          />
          <FrontendArchiveUrlField control={form.control} t={t} />
          <FrontendDownloadProxyField control={form.control} t={t} />
          <FrontendManifestUrlField control={form.control} t={t} />
          <p className='text-muted-foreground text-xs lg:col-span-2'>
            {t(
              'Updating replaces the current external frontend files without changing the embedded fallback.'
            )}
          </p>
        </SettingsForm>
      </Form>

      <UpdateDownloadCard
        job={job}
        staged={Boolean(stagedVersion) || staged}
        progressTitle={
          job?.source
            ? t('Downloading frontend from {{source}}', {
                source: job.source,
              })
            : t('Preparing frontend download…')
        }
        readyTitle={t('A new frontend is staged and ready to activate')}
        readyHint={t(
          'Activating swaps the live frontend files. Reload the page afterwards to load the new build.'
        )}
        activateLabel={t('Activate frontend')}
        discardLabel={t('Discard staged release')}
        onActivate={actions.handleActivate}
        onDiscard={actions.handleDiscard}
        discarding={actions.discarding}
      />

      <div className='grid gap-4 md:grid-cols-2'>
        <VersionValue
          label={t('Current frontend')}
          value={status?.current_version}
        />
        <VersionValue
          label={t('Latest checked frontend')}
          value={actions.latestCheckedVersion}
        />
        <VersionValue label={t('Pending activation')} value={stagedVersion} />
        <VersionValue
          label={t('Previous frontend')}
          value={status?.previous_version}
        />
      </div>

      <div className='flex flex-wrap gap-2'>
        <Button
          type='button'
          variant='outline'
          onClick={actions.handleCheck}
          disabled={
            actions.checking ||
            actions.starting ||
            actions.staging ||
            transferActive
          }
        >
          <RefreshCw
            data-icon='inline-start'
            className={actions.checking ? 'animate-spin' : undefined}
          />
          <span>
            {actions.checking
              ? t('Checking frontend...')
              : t('Check frontend updates')}
          </span>
        </Button>

        <Button
          type='button'
          onClick={actions.handleDownload}
          disabled={
            transferActive ||
            staged ||
            Boolean(stagedVersion) ||
            actions.starting ||
            actions.checking
          }
        >
          <Download data-icon='inline-start' />
          <span>
            {actions.starting
              ? t('Starting download...')
              : t('Download frontend update')}
          </span>
        </Button>

        <Button
          type='button'
          variant='destructive'
          onClick={actions.handleRollback}
          disabled={
            actions.staging ||
            actions.starting ||
            actions.checking ||
            transferActive ||
            Boolean(stagedVersion) ||
            !status?.can_rollback
          }
        >
          <RotateCcw data-icon='inline-start' />
          <span>{t('Stage previous version')}</span>
        </Button>
      </div>
    </SettingsSection>
  )
}
