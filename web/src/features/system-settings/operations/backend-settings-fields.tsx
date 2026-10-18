import type { Control } from 'react-hook-form'

import { Combobox } from '@/components/ui/combobox'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'

import { useProxyEntries } from '../hooks/use-proxy-entries'

export type BackendFormValues = {
  manifestUrl: string
  downloadProxy: string
}

type BackendFieldProps = {
  control: Control<BackendFormValues>
  t: (key: string) => string
}

export function BackendManifestUrlField(props: BackendFieldProps) {
  return (
    <FormField
      control={props.control}
      name='manifestUrl'
      render={({ field }) => (
        <FormItem className='lg:col-span-2'>
          <FormLabel>{props.t('Backend manifest URL')}</FormLabel>
          <FormControl>
            <Input
              type='url'
              inputMode='url'
              placeholder={props.t('Enter a signed backend manifest URL')}
              autoComplete='off'
              {...field}
            />
          </FormControl>
          <FormDescription>
            {props.t(
              'The signed manifest selects the verified Linux binary for this server architecture.'
            )}
          </FormDescription>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}

export function BackendDownloadProxyField(props: BackendFieldProps) {
  const proxyEntries = useProxyEntries()
  return (
    <FormField
      control={props.control}
      name='downloadProxy'
      render={({ field }) => (
        <FormItem className='lg:col-span-2'>
          <FormLabel>{props.t('Proxy Address')}</FormLabel>
          <FormControl>
            <Combobox
              allowCustomValue
              options={proxyEntries.map((entry) => ({
                value: entry.url,
                label: `${entry.name} — ${entry.url}`,
              }))}
              value={field.value || ''}
              onValueChange={(value) => field.onChange(value ?? '')}
              placeholder='http://127.0.0.1:7890'
              searchPlaceholder={props.t('Search...')}
            />
          </FormControl>
          <FormDescription>
            {props.t(
              'Optional proxy for backend manifest and binary downloads. Leave empty to use direct access or HTTP_PROXY/HTTPS_PROXY.'
            )}
          </FormDescription>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}
