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
import { render, screen } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterAll, beforeEach, expect, test, vi } from 'vitest'

import en from '@/i18n/locales/en.json'

import { PayloadDialog } from '../payload-dialog'

const getRequestPayload = vi.hoisted(() => vi.fn())
vi.mock('../../../api', () => ({ getRequestPayload }))

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
  getRequestPayload.mockReset()
  await i18n.init({
    lng: 'en',
    resources: { en },
    interpolation: { escapeValue: false },
  })
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
})

function renderDialog() {
  return render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <PayloadDialog
          requestId='req-1'
          isAdmin
          open
          onOpenChange={() => undefined}
        />
      </QueryClientProvider>
    </I18nextProvider>
  )
}

test('renders the captured request and response sections', async () => {
  getRequestPayload.mockResolvedValue({
    request_id: 'req-1',
    request_headers: '{"Authorization":"[REDACTED]"}',
    request_body: '{"messages":[]}',
    response_headers: '{"Content-Type":"application/json"}',
    response_body: '{"choices":[]}',
    is_truncated: false,
  })

  renderDialog()

  expect(await screen.findByText('Request Headers')).toBeVisible()
  expect(screen.getByText('Request Body')).toBeVisible()
  expect(screen.getByText('Response Headers')).toBeVisible()
  expect(screen.getByText('Response Body')).toBeVisible()
  expect(screen.getByText(/\[REDACTED\]/)).toBeVisible()
})

test('shows an error state when the payload is unavailable', async () => {
  getRequestPayload.mockRejectedValue(new Error('payload not found'))

  renderDialog()

  expect(
    await screen.findByText('Failed to load request details')
  ).toBeVisible()
})

test('marks truncated payloads', async () => {
  getRequestPayload.mockResolvedValue({
    request_id: 'req-1',
    request_headers: '{}',
    request_body: 'abc',
    response_headers: '{}',
    response_body: 'def',
    is_truncated: true,
  })

  renderDialog()

  // Both the request and response sections carry the truncated marker.
  expect(await screen.findAllByText('Truncated')).toHaveLength(2)
})
