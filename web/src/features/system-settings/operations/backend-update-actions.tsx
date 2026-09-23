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
import { Download, RefreshCw, RotateCcw } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

type BackendUpdateActionsProps = {
  checking: boolean
  starting: boolean
  staging: boolean
  /** A staged release disables both downloading and re-staging. */
  hasStagedRelease: boolean
  canRollback: boolean
  disabled: boolean
  onCheck: () => void
  onDownload: () => void
  onRollback: () => void
}

export function BackendUpdateActions(props: BackendUpdateActionsProps) {
  const { t } = useTranslation()
  const busy = props.disabled || props.checking || props.starting || props.staging

  return (
    <div className='flex flex-wrap gap-2'>
      <Button
        type='button'
        variant='outline'
        onClick={props.onCheck}
        disabled={busy}
      >
        <RefreshCw
          data-icon='inline-start'
          className={props.checking ? 'animate-spin' : undefined}
        />
        <span>
          {props.checking
            ? t('Checking backend...')
            : t('Check backend updates')}
        </span>
      </Button>

      <Button
        type='button'
        onClick={props.onDownload}
        disabled={busy || props.hasStagedRelease}
      >
        <Download data-icon='inline-start' />
        <span>
          {props.starting ? t('Starting download...') : t('Download update')}
        </span>
      </Button>

      <Button
        type='button'
        variant='destructive'
        onClick={props.onRollback}
        disabled={busy || props.hasStagedRelease || !props.canRollback}
      >
        <RotateCcw data-icon='inline-start' />
        <span>{t('Stage previous version')}</span>
      </Button>
    </div>
  )
}
