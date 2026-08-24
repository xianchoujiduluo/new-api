/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { zodResolver } from '@hookform/resolvers/zod'
import { RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Form } from '@/components/ui/form'

import { updateFrontend } from '../api'
import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'
import {
  FrontendArchiveUrlField,
  FrontendDownloadProxyField,
  type FrontendFormValues,
} from './frontend-settings-fields'
import { createFrontendSchema } from './frontend-settings-schema'

type FrontendSettingsSectionProps = {
  defaultDownloadUrl: string
  defaultDownloadProxy: string
}

export function FrontendSettingsSection(
  props: FrontendSettingsSectionProps
) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [isUpdating, setIsUpdating] = useState(false)
  const frontendSchema = createFrontendSchema(t)
  const defaultValues = {
    downloadUrl: props.defaultDownloadUrl ?? '',
    downloadProxy: props.defaultDownloadProxy ?? '',
  }

  const form = useForm<FrontendFormValues>({
    resolver: zodResolver(frontendSchema),
    defaultValues,
  })

  useResetForm(form, defaultValues)

  const saveSettings = async (values: FrontendFormValues) => {
    const currentUrl = (props.defaultDownloadUrl ?? '').trim()
    const currentProxy = (props.defaultDownloadProxy ?? '').trim()
    const downloadUrl = values.downloadUrl.trim()
    const downloadProxy = values.downloadProxy.trim()
    const updates = [
      ...(downloadUrl !== currentUrl
        ? [{ key: 'frontend_setting.download_url', value: downloadUrl }]
        : []),
      ...(downloadProxy !== currentProxy
        ? [{ key: 'frontend_setting.download_proxy', value: downloadProxy }]
        : []),
    ]

    for (const update of updates) {
      const result = await updateOption.mutateAsync(update)
      if (!result.success) return false
    }
    return true
  }

  const onSubmit = async (values: FrontendFormValues) => {
    const normalizedValues = {
      downloadUrl: values.downloadUrl.trim(),
      downloadProxy: values.downloadProxy.trim(),
    }
    const saved = await saveSettings(normalizedValues)
    if (saved) form.reset(normalizedValues)
  }

  const handleUpdate = async () => {
    const isValid = await form.trigger()
    if (!isValid) return

    const downloadUrl = form.getValues('downloadUrl').trim()
    if (!downloadUrl) {
      toast.error(t('Configure a frontend archive URL first'))
      return
    }

    setIsUpdating(true)
    try {
      const saved = await saveSettings({
        downloadUrl,
        downloadProxy: form.getValues('downloadProxy').trim(),
      })
      if (!saved) return

      const response = await updateFrontend()
      if (!response.success) {
        toast.error(response.message || t('Failed to update frontend'))
        return
      }

      toast.success(response.message || t('Frontend updated successfully'))
    } catch (error) {
      const message =
        error instanceof Error ? error.message : t('Failed to update frontend')
      toast.error(message)
    } finally {
      setIsUpdating(false)
    }
  }

  return (
    <SettingsSection title={t('Frontend')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)} autoComplete='off'>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending || isUpdating}
            saveLabel='Save Frontend settings'
          />
          <FrontendArchiveUrlField control={form.control} t={t} />
          <FrontendDownloadProxyField control={form.control} t={t} />
          <div className='flex items-center justify-between gap-4 lg:col-span-2'>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Updating replaces the current external frontend files without changing the embedded fallback.'
              )}
            </p>
            <Button
              type='button'
              variant='outline'
              onClick={handleUpdate}
              disabled={isUpdating || updateOption.isPending}
            >
              <RefreshCw
                data-icon='inline-start'
                className={isUpdating ? 'animate-spin' : undefined}
              />
              <span>
                {isUpdating ? t('Updating frontend...') : t('Update frontend')}
              </span>
            </Button>
          </div>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
