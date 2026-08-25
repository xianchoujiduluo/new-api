/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { zodResolver } from '@hookform/resolvers/zod'
import { RefreshCw, RotateCcw } from 'lucide-react'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import { Form } from '@/components/ui/form'

import {
  applyBackendUpdate,
  checkBackendUpdate,
  getBackendUpdateStatus,
  rollbackBackendUpdate,
} from '../api'
import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'
import {
  BackendDownloadProxyField,
  BackendManifestUrlField,
  type BackendFormValues,
} from './backend-settings-fields'
import { createBackendSchema } from './backend-settings-schema'
import { VersionValue } from './backend-settings-status'

type BackendSettingsSectionProps = {
  defaultManifestUrl: string
  defaultDownloadProxy: string
}

type PendingAction = 'apply' | 'rollback' | null
export function BackendSettingsSection(props: BackendSettingsSectionProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const updateOption = useUpdateOption()
  const [pendingAction, setPendingAction] = useState<PendingAction>(null)
  const [checking, setChecking] = useState(false)
  const [latestCheckedVersion, setLatestCheckedVersion] = useState('')
  const schema = createBackendSchema(t)
  const defaultValues = {
    manifestUrl: props.defaultManifestUrl ?? '',
    downloadProxy: props.defaultDownloadProxy ?? '',
  }
  const form = useForm<BackendFormValues>({
    resolver: zodResolver(schema),
    defaultValues,
  })
  useResetForm(form, defaultValues)

  const statusQuery = useQuery({
    queryKey: ['backend-update-status'],
    queryFn: getBackendUpdateStatus,
    refetchInterval: 30_000,
  })
  const status = statusQuery.data?.success ? statusQuery.data.data : undefined

  const saveSettings = async (values: BackendFormValues) => {
    const updates = [
      ...(values.manifestUrl.trim() !== props.defaultManifestUrl.trim()
        ? [{ key: 'backend_setting.manifest_url', value: values.manifestUrl.trim() }]
        : []),
      ...(values.downloadProxy.trim() !== props.defaultDownloadProxy.trim()
        ? [{ key: 'backend_setting.download_proxy', value: values.downloadProxy.trim() }]
        : []),
    ]
    for (const update of updates) {
      const response = await updateOption.mutateAsync(update)
      if (!response.success) return false
    }
    return true
  }

  const onSubmit = async (values: BackendFormValues) => {
    const normalized = {
      manifestUrl: values.manifestUrl.trim(),
      downloadProxy: values.downloadProxy.trim(),
    }
    if (await saveSettings(normalized)) form.reset(normalized)
  }

  const handleCheck = async () => {
    setChecking(true)
    try {
      const saved = await saveSettings({
        manifestUrl: form.getValues('manifestUrl').trim(),
        downloadProxy: form.getValues('downloadProxy').trim(),
      })
      if (!saved) return
      const response = await checkBackendUpdate()
      if (!response.success || !response.data) {
        toast.error(response.message || t('Failed to check backend updates'))
        return
      }
      setLatestCheckedVersion(response.data.latest_version)
      await queryClient.invalidateQueries({ queryKey: ['backend-update-status'] })
      toast.success(
        response.data.update_available
          ? t('A backend update is available')
          : t('Backend is up to date')
      )
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Failed to check backend updates'))
    } finally {
      setChecking(false)
    }
  }

  const handleAction = async () => {
    if (!pendingAction) return
    try {
      const response =
        pendingAction === 'apply'
          ? await applyBackendUpdate()
          : await rollbackBackendUpdate()
      if (!response.success) {
        toast.error(response.message || t('Backend operation failed'))
        return
      }
      toast.success(t('Backend restart has been requested'))
      await queryClient.invalidateQueries({ queryKey: ['backend-update-status'] })
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Backend operation failed'))
    } finally {
      setPendingAction(null)
    }
  }

  const managed = Boolean(status?.supported && status.supervised)
  const operationDisabled = !managed || statusQuery.isLoading

  return (
    <>
      <SettingsSection title={t('Backend updates')}>
        <Form {...form}>
          <SettingsForm onSubmit={form.handleSubmit(onSubmit)} autoComplete='off'>
            <SettingsPageFormActions
              onSave={form.handleSubmit(onSubmit)}
              isSaving={updateOption.isPending || checking}
              saveLabel='Save backend settings'
            />
            <BackendManifestUrlField control={form.control} t={t} />
            <BackendDownloadProxyField control={form.control} t={t} />
          </SettingsForm>
        </Form>

        <div className='grid gap-4 md:grid-cols-2'>
          <VersionValue label={t('Current backend')} value={status?.current_version} />
          <VersionValue label={t('Latest checked backend')} value={latestCheckedVersion} />
          <VersionValue label={t('Pending restart')} value={status?.pending_version} />
          <VersionValue label={t('Previous backend')} value={status?.previous_version} />
        </div>

        {!managed && (
          <p className='text-muted-foreground text-sm'>
            {t('Backend self-update requires the launcher-enabled Docker image.')}
          </p>
        )}
        <div className='flex flex-wrap gap-2'>
          <Button type='button' variant='outline' onClick={handleCheck} disabled={checking}>
            <RefreshCw data-icon='inline-start' className={checking ? 'animate-spin' : undefined} />
            <span>{checking ? t('Checking backend...') : t('Check backend updates')}</span>
          </Button>
          <Button type='button' onClick={() => setPendingAction('apply')} disabled={operationDisabled}>
            <RefreshCw data-icon='inline-start' />
            <span>{t('Update and restart')}</span>
          </Button>
          <Button type='button' variant='destructive' onClick={() => setPendingAction('rollback')} disabled={operationDisabled || !status?.can_rollback}>
            <RotateCcw data-icon='inline-start' />
            <span>{t('Rollback and restart')}</span>
          </Button>
        </div>
      </SettingsSection>

      <ConfirmDialog
        open={pendingAction !== null}
        onOpenChange={(open) => !open && setPendingAction(null)}
        title={pendingAction === 'apply' ? t('Confirm backend update') : t('Confirm backend rollback')}
        desc={pendingAction === 'apply' ? t('The server will download a signed binary and restart.') : t('The server will restart using the previous backend version.')}
        confirmText={pendingAction === 'apply' ? t('Update and restart') : t('Rollback and restart')}
        destructive={pendingAction === 'rollback'}
        handleConfirm={handleAction}
      />
    </>
  )
}
