import { useQuery } from '@tanstack/react-query'

import { requireServerSuccess } from '@/lib/server-error-message'

import { getSystemOptions } from '../api'

export type ProxyEntry = {
  id: string
  name: string
  url: string
  enabled?: boolean
}

/**
 * Reads the shared proxy registry from the persisted system options. The
 * registry is the single place an operator maintains proxy URLs, so update
 * download proxies can be picked instead of typed.
 */
export function useProxyEntries(): ProxyEntry[] {
  const { data } = useQuery({
    queryKey: ['proxy_setting_options'],
    queryFn: async () => requireServerSuccess(await getSystemOptions()),
  })
  // /api/option/ returns {key, value}[] where value is a JSON string.
  const raw = data?.data?.find(
    (option) => option.key === 'proxy_setting.proxies'
  )?.value
  if (!raw) return []
  try {
    const list = JSON.parse(raw)
    if (!Array.isArray(list)) return []
    return list.filter(
      (entry): entry is ProxyEntry =>
        Boolean(entry) &&
        typeof entry.id === 'string' &&
        typeof entry.url === 'string' &&
        entry.url.trim() !== '' &&
        entry.enabled !== false
    )
  } catch {
    return []
  }
}
