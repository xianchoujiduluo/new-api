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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { renderHook } from '@testing-library/react'
import { createInstance } from 'i18next'
import type { ReactNode } from 'react'
import { I18nextProvider } from 'react-i18next'
import { afterAll, beforeEach, expect, test, vi } from 'vitest'

import en from '@/i18n/locales/en.json'

import { useCommonLogsColumns } from '../columns/common-logs-columns'

vi.mock('@lobehub/icons', () => ({}))
vi.hoisted(() => {
  vi.stubGlobal('localStorage', {
    getItem: () => null,
    setItem: () => undefined,
    removeItem: () => undefined,
  })
})
afterAll(() => vi.unstubAllGlobals())

let client: QueryClient
const i18n = createInstance()
beforeEach(async () => {
  await i18n.init({
    lng: 'en',
    resources: { en },
    interpolation: { escapeValue: false },
  })
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
})

function wrapper(props: { children: ReactNode }) {
  return (
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        {props.children}
      </QueryClientProvider>
    </I18nextProvider>
  )
}

test('omits the request-details column when payload recording is off', () => {
  const { result } = renderHook(
    () => useCommonLogsColumns(true, false, false),
    {
      wrapper,
    }
  )

  expect(result.current.some((c) => c.id === 'payload')).toBe(false)
})

test('adds the request-details column when a payload was recorded', () => {
  const { result } = renderHook(() => useCommonLogsColumns(true, false, true), {
    wrapper,
  })

  expect(result.current.some((c) => c.id === 'payload')).toBe(true)
})
