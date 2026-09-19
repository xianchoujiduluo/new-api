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
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { render, screen } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterAll, beforeEach, expect, test, vi } from 'vitest'

import en from '@/i18n/locales/en.json'

import type { UsageLog } from '../../data/schema'
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

function makeLog(override: Partial<UsageLog>): UsageLog {
  return {
    id: 1,
    user_id: 1,
    created_at: 1,
    type: 2,
    content: '',
    username: 'user',
    token_name: 'token',
    model_name: 'gpt-4o',
    quota: 1,
    prompt_tokens: 0,
    completion_tokens: 0,
    use_time: 0,
    is_stream: false,
    channel: 1,
    channel_name: '',
    token_id: 1,
    group: 'default',
    ip: '',
    other: '{}',
    request_id: 'req-1',
    upstream_request_id: '',
    ...override,
  }
}

function IpCell(props: { log: UsageLog }) {
  const table = useReactTable({
    data: [props.log],
    columns: useCommonLogsColumns(false, false),
    getCoreRowModel: getCoreRowModel(),
  })
  const cell = table
    .getRowModel()
    .rows[0].getAllCells()
    .find((item) => item.column.id === 'ip')
  if (!cell) throw new Error('The log must have an ip column')
  return flexRender(cell.column.columnDef.cell, cell.getContext())
}

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

function renderIp(log: UsageLog) {
  return render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <IpCell log={log} />
      </QueryClientProvider>
    </I18nextProvider>
  )
}

test('renders the recorded IP for a usage log', () => {
  renderIp(makeLog({ type: 2, ip: '203.0.113.7' }))

  expect(screen.getByText('203.0.113.7')).toBeVisible()
})

test('renders the recorded IP for an error log', () => {
  renderIp(makeLog({ type: 5, ip: '203.0.113.8' }))

  expect(screen.getByText('203.0.113.8')).toBeVisible()
})

test('renders nothing for a usage log without a recorded IP', () => {
  const { container } = renderIp(makeLog({ type: 2, ip: '' }))

  expect(container).toBeEmptyDOMElement()
})

test('renders nothing for log types that never record an IP', () => {
  const { container } = renderIp(makeLog({ type: 1, ip: '203.0.113.9' }))

  expect(container).toBeEmptyDOMElement()
})
