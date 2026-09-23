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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import type { UseFormReturn } from 'react-hook-form'

import {
  activateFrontend,
  discardFrontendDownload,
  startFrontendDownload,
} from '../api'
import { useUpdateOption } from '../hooks/use-update-option'
import type { FrontendFormValues } from './frontend-settings-fields'

type FrontendUpdateActionsOptions = {
  form: UseFormReturn<FrontendFormValues>
  defaults: { downloadUrl: string; downloadProxy: string }
  refresh: () => Promise<void>
}

/**
 * Owns the frontend update action state. Staging downloads and extracts the
 * archive; activation swaps live assets. Splitting them keeps the slow transfer
 * observable and the disruptive step explicit.
 */
export function useFrontendUpdateActions(
  options: FrontendUpdateActionsOptions
) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [starting, setStarting] = useState(false)
  const [activating, setActivating] = useState(false)
  const [discarding, setDiscarding] = useState(false)

  const reportError = (error: unknown, fallback: string) => {
    toast.error(error instanceof Error ? error.message : fallback)
  }

  const pendingUpdates = (values: FrontendFormValues) => {
    const downloadUrl = values.downloadUrl.trim()
    const downloadProxy = values.downloadProxy.trim()
    return [
      ...(downloadUrl !== options.defaults.downloadUrl.trim()
        ? [{ key: 'frontend_setting.download_url', value: downloadUrl }]
        : []),
      ...(downloadProxy !== options.defaults.downloadProxy.trim()
        ? [{ key: 'frontend_setting.download_proxy', value: downloadProxy }]
        : []),
    ]
  }

  const applyUpdates = async (
    updates: { key: string; value: string }[]
  ): Promise<boolean> => {
    for (const update of updates) {
      const response = await updateOption.mutateAsync(update)
      if (!response.success) return false
    }
    return true
  }

  const handleSave = async (values: FrontendFormValues) => {
    if (!(await applyUpdates(pendingUpdates(values)))) return
    options.form.reset({
      downloadUrl: values.downloadUrl.trim(),
      downloadProxy: values.downloadProxy.trim(),
    })
  }

  // Step 1: stage the archive in the background and observe its progress.
  const handleDownload = async () => {
    const values = {
      downloadUrl: options.form.getValues('downloadUrl'),
      downloadProxy: options.form.getValues('downloadProxy'),
    }
    if (!values.downloadUrl.trim()) {
      toast.error(t('Configure a frontend archive URL first'))
      return
    }
    setStarting(true)
    try {
      if (!(await applyUpdates(pendingUpdates(values)))) return
      const response = await startFrontendDownload()
      if (!response.success && response.message) {
        toast.error(response.message)
        return
      }
      await options.refresh()
    } catch (error) {
      reportError(error, t('Failed to update frontend'))
    } finally {
      setStarting(false)
    }
  }

  // Step 2: the operator decides when the staged frontend goes live.
  const handleActivate = async () => {
    setActivating(true)
    try {
      const response = await activateFrontend()
      if (!response.success) {
        toast.error(response.message || t('Failed to update frontend'))
        return
      }
      toast.success(t('Frontend activated. Reload the page to see it.'))
      await options.refresh()
    } catch (error) {
      reportError(error, t('Failed to update frontend'))
    } finally {
      setActivating(false)
    }
  }

  const handleDiscard = async () => {
    setDiscarding(true)
    try {
      const response = await discardFrontendDownload()
      if (!response.success) {
        toast.error(response.message || t('Failed to update frontend'))
        return
      }
      toast.success(t('Staged release discarded'))
      await options.refresh()
    } catch (error) {
      reportError(error, t('Failed to update frontend'))
    } finally {
      setDiscarding(false)
    }
  }

  return {
    starting,
    activating,
    discarding,
    savingSettings: updateOption.isPending,
    handleSave,
    handleDownload,
    handleActivate,
    handleDiscard,
  }
}
