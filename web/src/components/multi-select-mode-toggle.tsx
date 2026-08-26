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
import { useTranslation } from 'react-i18next'

import { Switch } from '@/components/ui/switch'

interface MultiSelectModeToggleProps {
  multiple: boolean
  onMultipleChange: (multiple: boolean) => void
}

export function MultiSelectModeToggle(props: MultiSelectModeToggleProps) {
  const { t } = useTranslation()
  const label = props.multiple ? t('Multiple') : t('Single')

  return (
    <div
      className='border-border ml-1 flex shrink-0 items-center gap-1.5 border-l pl-2'
      title={label}
      onClick={(event) => event.stopPropagation()}
      onPointerDown={(event) => event.stopPropagation()}
    >
      <span className='text-muted-foreground max-w-14 truncate text-[11px] leading-none font-medium'>
        {label}
      </span>
      <Switch
        size='sm'
        checked={props.multiple}
        onCheckedChange={props.onMultipleChange}
        aria-label={label}
        className='cursor-pointer'
      />
    </div>
  )
}
