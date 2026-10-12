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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { PlusIcon, Trash2Icon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { handleServerError } from '@/lib/handle-server-error'

import { getSystemOptions, updateSystemOption } from '../api'
import { SettingsSection } from '../components/settings-section'

const OPTION_KEY = 'proxy_setting.proxies'

type ProxyEntry = {
  id: string
  name: string
  url: string
  enabled: boolean
}

function newProxyId(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) {
    return crypto.randomUUID()
  }
  return `p${Date.now()}${Math.floor(Math.random() * 1000)}`
}

function parseProxies(raw?: string): ProxyEntry[] {
  if (!raw) return []
  try {
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed.map((entry) => ({
      id: String(entry?.id ?? ''),
      name: String(entry?.name ?? ''),
      url: String(entry?.url ?? ''),
      enabled: entry?.enabled !== false,
    }))
  } catch {
    return []
  }
}

/**
 * Registry of named proxies that channels reference by id, so editing an entry
 * applies everywhere it is used instead of having to retype the address per
 * channel. Entries may carry credentials, so this section is only reachable
 * through root-only settings.
 */
export function ProxyRegistrySection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [draft, setDraft] = useState<ProxyEntry[] | null>(null)
  const [saving, setSaving] = useState(false)

  const { data, isLoading } = useQuery({
    queryKey: ['proxy-registry-options'],
    queryFn: async () => getSystemOptions(),
  })
  const stored = parseProxies(
    data?.data?.find((option) => option.key === OPTION_KEY)?.value
  )
  // The draft is null until the operator edits, so a refetch never clobbers
  // in-progress input while untouched rows keep tracking the server value.
  const proxies = draft ?? stored

  const update = (next: ProxyEntry[]) => setDraft(next)

  const handleSave = async () => {
    setSaving(true)
    try {
      const response = await updateSystemOption({
        key: OPTION_KEY,
        value: JSON.stringify(proxies),
      })
      if (!response.success) {
        toast.error(response.message || t('Failed to save proxy settings'))
        return
      }
      toast.success(t('Proxy settings saved'))
      setDraft(null)
      await queryClient.invalidateQueries({ queryKey: ['proxy-registry-options'] })
      await queryClient.invalidateQueries({ queryKey: ['proxy_setting_options'] })
    } catch (error) {
      handleServerError(error)
    } finally {
      setSaving(false)
    }
  }

  return (
    <SettingsSection
      title={t('Proxy Registry')}
      titleProps={{ id: 'proxy-registry' }}
    >
      <div className='space-y-3'>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Define named proxies once, then pick them in a channel. Editing an entry applies to every channel that references it. A disabled entry stops applying and those channels fall back to a direct connection.'
          )}
        </p>

        {isLoading && (
          <p className='text-muted-foreground text-sm'>{t('Loading...')}</p>
        )}

        <div className='space-y-2'>
          {proxies.map((entry, index) => (
            <div
              key={entry.id || index}
              className='flex flex-wrap items-center gap-2 rounded-lg border p-2'
            >
              <Input
                className='w-40'
                placeholder={t('Name')}
                value={entry.name}
                onChange={(event) =>
                  update(
                    proxies.map((item, itemIndex) =>
                      itemIndex === index
                        ? { ...item, name: event.target.value }
                        : item
                    )
                  )
                }
              />
              <Input
                className='min-w-56 flex-1'
                placeholder={t('socks5://user:pass@host:port')}
                value={entry.url}
                onChange={(event) =>
                  update(
                    proxies.map((item, itemIndex) =>
                      itemIndex === index
                        ? { ...item, url: event.target.value }
                        : item
                    )
                  )
                }
              />
              <div className='flex items-center gap-2'>
                <Switch
                  checked={entry.enabled}
                  onCheckedChange={(checked) =>
                    update(
                      proxies.map((item, itemIndex) =>
                        itemIndex === index
                          ? { ...item, enabled: checked }
                          : item
                      )
                    )
                  }
                />
                <span className='text-muted-foreground text-xs'>
                  {t('Enabled')}
                </span>
              </div>
              <Button
                type='button'
                variant='ghost'
                size='icon'
                aria-label={t('Delete')}
                onClick={() =>
                  update(proxies.filter((_, itemIndex) => itemIndex !== index))
                }
              >
                <Trash2Icon className='h-4 w-4' />
              </Button>
            </div>
          ))}
        </div>

        <div className='flex flex-wrap gap-2'>
          <Button
            type='button'
            variant='outline'
            onClick={() =>
              update([
                ...proxies,
                { id: newProxyId(), name: '', url: '', enabled: true },
              ])
            }
          >
            <PlusIcon data-icon='inline-start' />
            {t('Add proxy')}
          </Button>
          <Button type='button' disabled={saving || draft === null} onClick={handleSave}>
            {saving ? t('Saving...') : t('Save')}
          </Button>
        </div>

        <p className='text-muted-foreground text-xs'>
          {t(
            'A proxy that a channel still references cannot be removed; change those channels first.'
          )}
        </p>
      </div>
    </SettingsSection>
  )
}
