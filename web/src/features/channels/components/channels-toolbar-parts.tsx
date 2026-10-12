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
import { Eye, EyeOff } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'

import { CHANNEL_STATUS_OPTIONS } from '../constants'
import type { Channel } from '../types'
import { useChannels } from './channels-provider'

// 状态直接以内联按钮组列出，默认「所有状态」，省去展开下拉再选一次的操作。
export function ChannelStatusFilter(props: { table: Table<Channel> }) {
  const { t } = useTranslation()
  const column = props.table.getColumn('status')
  const selectedValues = column?.getFilterValue() as string[] | undefined
  const selectedValue = selectedValues?.[0] ?? 'all'

  return (
    <div className='flex max-w-full min-w-0 flex-wrap items-center gap-2'>
      <span className='text-muted-foreground text-sm'>{t('Status')}</span>
      <ToggleGroup
        value={[selectedValue]}
        onValueChange={(values) => {
          const value = values[0] ?? 'all'
          column?.setFilterValue(value === 'all' ? undefined : [value])
        }}
        multiple={false}
        variant='outline'
        spacing={1}
        className='max-w-full flex-wrap'
        aria-label={t('Status')}
      >
        {CHANNEL_STATUS_OPTIONS.map((option) => (
          <ToggleGroupItem
            key={option.value}
            value={option.value}
            className='aria-pressed:border-primary/30 aria-pressed:bg-primary/10 aria-pressed:text-primary'
          >
            {t(option.label)}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
    </div>
  )
}

export function ChannelVisibilityToggle() {
  const { t } = useTranslation()
  const { sensitiveVisible, setSensitiveVisible } = useChannels()
  const label = sensitiveVisible ? t('Hide') : t('Show')

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            variant='ghost'
            size='icon'
            onClick={() => setSensitiveVisible(!sensitiveVisible)}
            aria-label={label}
            className='text-muted-foreground hover:text-foreground size-8'
          />
        }
      >
        {sensitiveVisible ? <Eye /> : <EyeOff />}
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  )
}
