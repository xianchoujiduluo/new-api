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
import { useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import type { UseFormReturn } from 'react-hook-form'

import {
  checkBackendUpdate,
  discardBackendDownload,
  restartBackend,
  rollbackBackendUpdate,
  startBackendDownload,
} from '../api'
import { useUpdateOption } from '../hooks/use-update-option'
import type { BackendFormValues } from './backend-settings-fields'

type BackendUpdateActionsOptions = {
  form: UseFormReturn<BackendFormValues>
  defaults: { manifestUrl: string; downloadProxy: string }
  refresh: () => Promise<void>
  beginRestart: () => void
}

/**
 * Owns the backend update action state. Download and restart are intentionally
 * separate actions: staging a release never restarts the process on its own, so
 * the operator stays in control of when the interruption happens.
 */
export function useBackendUpdateActions(options: BackendUpdateActionsOptions) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const updateOption = useUpdateOption()
  const [checking, setChecking] = useState(false)
  const [starting, setStarting] = useState(false)
  const [staging, setStaging] = useState(false)
  const [discarding, setDiscarding] = useState(false)
  const [latestCheckedVersion, setLatestCheckedVersion] = useState('')

  const reportError = (error: unknown, fallback: string) => {
    toast.error(error instanceof Error ? error.message : fallback)
  }

  // The manifest URL and proxy must be persisted before any action uses them.
  const applyUpdates = async (
    updates: { key: string; value: string }[]
  ): Promise<boolean> => {
    for (const update of updates) {
      const response = await updateOption.mutateAsync(update)
      if (!response.success) return false
    }
    return true
  }

  const pendingUpdates = (values: BackendFormValues) => {
    const manifestUrl = values.manifestUrl.trim()
    const downloadProxy = values.downloadProxy.trim()
    return [
      ...(manifestUrl !== options.defaults.manifestUrl.trim()
        ? [{ key: 'backend_setting.manifest_url', value: manifestUrl }]
        : []),
      ...(downloadProxy !== options.defaults.downloadProxy.trim()
        ? [{ key: 'backend_setting.download_proxy', value: downloadProxy }]
        : []),
    ]
  }

  const saveSettings = async (): Promise<boolean> =>
    applyUpdates(
      pendingUpdates({
        manifestUrl: options.form.getValues('manifestUrl'),
        downloadProxy: options.form.getValues('downloadProxy'),
      })
    )

  const handleSave = async (values: BackendFormValues) => {
    if (!(await applyUpdates(pendingUpdates(values)))) return
    options.form.reset({
      manifestUrl: values.manifestUrl.trim(),
      downloadProxy: values.downloadProxy.trim(),
    })
  }

  const handleCheck = async () => {
    setChecking(true)
    try {
      if (!(await saveSettings())) return
      const response = await checkBackendUpdate()
      if (!response.success || !response.data) {
        toast.error(response.message || t('Failed to check backend updates'))
        return
      }
      setLatestCheckedVersion(response.data.latest_version)
      await options.refresh()
      toast.success(
        response.data.update_available
          ? t('A backend update is available')
          : t('Backend is up to date')
      )
    } catch (error) {
      reportError(error, t('Failed to check backend updates'))
    } finally {
      setChecking(false)
    }
  }

  // Step 1 of the update: stage the release and let the UI observe progress.
  const handleDownload = async () => {
    setStarting(true)
    try {
      if (!(await saveSettings())) return
      const response = await startBackendDownload()
      if (!response.success && response.message) {
        toast.error(response.message)
        return
      }
      await queryClient.invalidateQueries({ queryKey: ['backend-download-job'] })
    } catch (error) {
      reportError(error, t('Backend operation failed'))
    } finally {
      setStarting(false)
    }
  }

  // Rollback only stages the previous release; activation reuses the same
  // restart action, so both paths share one confirmation point.
  const handleRollback = async () => {
    setStaging(true)
    try {
      const response = await rollbackBackendUpdate()
      if (!response.success) {
        toast.error(response.message || t('Backend operation failed'))
        return
      }
      toast.success(t('Previous backend staged. Restart to activate it.'))
      await options.refresh()
    } catch (error) {
      reportError(error, t('Backend operation failed'))
    } finally {
      setStaging(false)
    }
  }

  // Dropping a staged release avoids a restart-and-rollback cycle when the
  // wrong version was downloaded.
  const handleDiscard = async () => {
    setDiscarding(true)
    try {
      const response = await discardBackendDownload()
      if (!response.success) {
        toast.error(response.message || t('Backend operation failed'))
        return
      }
      toast.success(t('Staged release discarded'))
      await options.refresh()
    } catch (error) {
      reportError(error, t('Backend operation failed'))
    } finally {
      setDiscarding(false)
    }
  }

  const handleRestart = async () => {
    try {
      const response = await restartBackend()
      if (!response.success) {
        toast.error(response.message || t('Backend operation failed'))
        return
      }
      options.beginRestart()
    } catch (error) {
      reportError(error, t('Backend operation failed'))
    }
  }

  return {
    checking,
    starting,
    staging,
    discarding,
    latestCheckedVersion,
    savingSettings: updateOption.isPending,
    handleSave,
    handleCheck,
    handleDownload,
    handleRollback,
    handleDiscard,
    handleRestart,
  }
}
