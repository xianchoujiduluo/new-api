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
import type { Control } from 'react-hook-form'

import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'

export type FrontendFormValues = {
  downloadUrl: string
  downloadProxy: string
}

type FrontendFieldProps = {
  control: Control<FrontendFormValues>
  t: (key: string) => string
}

export function FrontendArchiveUrlField({ control, t }: FrontendFieldProps) {
  return (
    <FormField
      control={control}
      name='downloadUrl'
      render={({ field }) => (
        <FormItem className='lg:col-span-2'>
          <FormLabel>{t('Frontend archive URL')}</FormLabel>
          <FormControl>
            <Input
              type='url'
              inputMode='url'
              placeholder={t('Enter a frontend archive URL')}
              autoComplete='off'
              {...field}
            />
          </FormControl>
          <FormDescription>
            {t(
              'Leave empty to use the embedded frontend. The archive must contain an index.html file.'
            )}
          </FormDescription>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}

export function FrontendDownloadProxyField({ control, t }: FrontendFieldProps) {
  return (
    <FormField
      control={control}
      name='downloadProxy'
      render={({ field }) => (
        <FormItem className='lg:col-span-2'>
          <FormLabel>{t('Proxy Address')}</FormLabel>
          <FormControl>
            <Input
              type='text'
              inputMode='url'
              placeholder='http://127.0.0.1:7890'
              autoComplete='off'
              {...field}
            />
          </FormControl>
          <FormDescription>
            {t(
              'Optional proxy for frontend archive downloads. Leave empty to use direct access or HTTP_PROXY/HTTPS_PROXY.'
            )}
          </FormDescription>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}
