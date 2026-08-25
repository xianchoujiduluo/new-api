import * as z from 'zod'

import {
  isOptionalDownloadURL,
  isOptionalProxyURL,
} from './frontend-settings-schema'

export function createBackendSchema(t: (key: string) => string) {
  return z.object({
    manifestUrl: z
      .string()
      .refine(
        isOptionalDownloadURL,
        t('Provide a valid URL starting with http:// or https://')
      ),
    downloadProxy: z
      .string()
      .refine(
        isOptionalProxyURL,
        t(
          'Proxy address must use HTTP, HTTPS, SOCKS5, or SOCKS5H and include a valid host'
        )
      ),
  })
}
