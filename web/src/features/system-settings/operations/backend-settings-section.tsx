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
import { useCallback } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { Form } from '@/components/ui/form'

import { getBackendDownloadJob, getBackendUpdateStatus } from '../api'
import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { UpdateDownloadCard } from '../components/update-download-card'
import { BackendUpdateActions } from './backend-update-actions'
import {
  BackendDownloadProxyField,
  BackendManifestUrlField,
  type BackendFormValues,
} from './backend-settings-fields'
import { createBackendSchema } from './backend-settings-schema'
import { VersionValue } from './backend-settings-status'
import { useBackendRestartWatch } from './use-backend-restart'
import { useBackendUpdateActions } from './use-backend-update-actions'

const TRANSFER_STATES = [
  'fetching',
  'downloading',
  'verifying',
  'installing',
] as const

type BackendSettingsSectionProps = {
  defaultManifestUrl: string
  defaultDownloadProxy: string
}

export function BackendSettingsSection(props: BackendSettingsSectionProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const defaults = {
    manifestUrl: props.defaultManifestUrl ?? '',
    downloadProxy: props.defaultDownloadProxy ?? '',
  }
  const form = useForm<BackendFormValues>({
    resolver: zodResolver(createBackendSchema(t)),
    defaultValues: defaults,
  })
  useResetForm(form, defaults)

  const statusQuery = useQuery({
    queryKey: ['backend-update-status'],
    queryFn: getBackendUpdateStatus,
    refetchInterval: 30_000,
  })
  const status = statusQuery.data?.success ? statusQuery.data.data : undefined
  const stagedVersion = status?.pending_version || undefined

  const refresh = useCallback(async () => {
    await queryClient.invalidateQueries({ queryKey: ['backend-update-status'] })
    await queryClient.invalidateQueries({ queryKey: ['backend-download-job'] })
  }, [queryClient])

  const { restarting, seconds, begin } = useBackendRestartWatch(refresh)

  const actions = useBackendUpdateActions({ form, defaults, refresh, beginRestart: begin })

  // Poll only while a transfer is running; an idle page stays quiet.
  const jobQuery = useQuery({
    queryKey: ['backend-download-job'],
    queryFn: getBackendDownloadJob,
    refetchInterval: (query) => {
      const state = query.state.data?.data?.state
      if (!state) return false
      return (TRANSFER_STATES as readonly string[]).includes(state) ? 1_000 : false
    },
  })
  const job = jobQuery.data?.success ? jobQuery.data.data : undefined
  const transferActive = (TRANSFER_STATES as readonly string[]).includes(
    job?.state ?? ''
  )

  const managed = Boolean(status?.supported && status.supervised)

  return (
    <SettingsSection title={t('Backend updates')}>
      <Form {...form}>
        <SettingsForm
          onSubmit={form.handleSubmit(actions.handleSave)}
          autoComplete='off'
        >
          <SettingsPageFormActions
            onSave={form.handleSubmit(actions.handleSave)}
            isSaving={actions.savingSettings || actions.checking}
            saveLabel='Save backend settings'
          />
          <BackendManifestUrlField control={form.control} t={t} />
          <BackendDownloadProxyField control={form.control} t={t} />
        </SettingsForm>
      </Form>

      <div className='grid gap-4 md:grid-cols-2'>
        <VersionValue
          label={t('Current backend')}
          value={status?.current_version}
        />
        <VersionValue
          label={t('Latest checked backend')}
          value={actions.latestCheckedVersion}
        />
        <VersionValue label={t('Pending restart')} value={stagedVersion} />
        <VersionValue
          label={t('Previous backend')}
          value={status?.previous_version}
        />
      </div>

      <UpdateDownloadCard
        job={job}
        staged={Boolean(stagedVersion)}
        progressTitle={
          job?.version
            ? t('Downloading backend {{version}}', { version: job.version })
            : t('Preparing backend download…')
        }
        readyTitle={
          stagedVersion
            ? t('Backend {{version}} is ready to activate', {
                version: stagedVersion,
              })
            : t('A backend release is ready to activate')
        }
        readyHint={t(
          'Restarting takes effect immediately and interrupts in-flight requests.'
        )}
        activateLabel={t('Restart and activate')}
        discardLabel={t('Discard staged release')}
        onActivate={actions.handleRestart}
        onDiscard={actions.handleDiscard}
        discarding={actions.discarding}
        activating={
          restarting
            ? { label: t('Restarting the backend…'), seconds }
            : undefined
        }
      />

      {!managed && (
        <p className='text-muted-foreground text-sm'>
          {t('Backend self-update requires the launcher-enabled Docker image.')}
        </p>
      )}

      <BackendUpdateActions
        checking={actions.checking}
        starting={actions.starting}
        staging={actions.staging}
        hasStagedRelease={Boolean(stagedVersion)}
        canRollback={Boolean(status?.can_rollback)}
        disabled={!managed || restarting || transferActive}
        onCheck={actions.handleCheck}
        onDownload={actions.handleDownload}
        onRollback={actions.handleRollback}
      />
    </SettingsSection>
  )
}
