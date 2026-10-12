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
import type { Table } from '@tanstack/react-table'
import type { ComponentProps } from 'react'
import { useTranslation } from 'react-i18next'

import type { DataTablePageProps } from '@/components/data-table'
import { Input } from '@/components/ui/input'

import type { Channel } from '../types'
import { ChannelStatusFilter, ChannelVisibilityToggle } from './channels-toolbar-parts'

type ToolbarProps = NonNullable<DataTablePageProps<Channel>['toolbarProps']>
type FilterOptions = NonNullable<ToolbarProps['filters']>[number]['options']

interface ChannelsToolbarOptions {
  table: Table<Channel>
  modelInputProps: Pick<
    ComponentProps<typeof Input>,
    'value' | 'onChange' | 'onCompositionStart' | 'onCompositionEnd'
  >
  onReset: () => void
  typeFilterOptions: FilterOptions
  groupFilterOptions: FilterOptions
}

export function useChannelsToolbar(
  props: ChannelsToolbarOptions
): ToolbarProps {
  const { t } = useTranslation()

  return {
    searchPlaceholder: t('Filter by name, ID, or key...'),
    searchDebounceMs: 500,
    onReset: props.onReset,
    additionalSearch: (
      <>
        <Input
          {...props.modelInputProps}
          placeholder={t('Filter by model...')}
          className='w-full sm:w-[150px] lg:w-[180px]'
        />
        <ChannelStatusFilter table={props.table} />
      </>
    ),
    filters: [
      {
        columnId: 'type',
        title: t('Type'),
        options: props.typeFilterOptions,
        singleSelect: true,
      },
      {
        columnId: 'group',
        title: t('Group'),
        options: props.groupFilterOptions,
        singleSelect: true,
      },
    ],
    preActions: <ChannelVisibilityToggle />,
  }
}
