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
import * as z from 'zod'

const PROXY_PROTOCOLS = new Set(['http:', 'https:', 'socks5:', 'socks5h:'])
const DOWNLOAD_PROTOCOLS = new Set(['http:', 'https:'])

function isOptionalURL(value: string, protocols: Set<string>) {
  const trimmed = value.trim()
  if (!trimmed) return true
  try {
    const parsed = new URL(trimmed)
    return protocols.has(parsed.protocol) && Boolean(parsed.hostname)
  } catch {
    return false
  }
}

function isOptionalProxyURL(value: string) {
  const trimmed = value.trim()
  if (!isOptionalURL(trimmed, PROXY_PROTOCOLS)) return false
  if (!trimmed) return true
  const schemeSeparator = trimmed.indexOf('://')
  const authorityAndSuffix = trimmed.slice(schemeSeparator + 3)
  const suffixIndex = authorityAndSuffix.search(/[/?#]/)
  if (suffixIndex >= 0 && authorityAndSuffix.slice(suffixIndex) !== '/') {
    return false
  }
  const parsed = new URL(trimmed)
  return (
    (parsed.pathname === '' || parsed.pathname === '/') &&
    !parsed.search &&
    !parsed.hash &&
    parsed.port !== '0'
  )
}

function isOptionalDownloadURL(value: string) {
  const trimmed = value.trim()
  if (!isOptionalURL(trimmed, DOWNLOAD_PROTOCOLS)) return false
  if (!trimmed) return true
  const parsed = new URL(trimmed)
  return !parsed.username && !parsed.password
}

export function createFrontendSchema(t: (key: string) => string) {
  return z.object({
    downloadUrl: z
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
