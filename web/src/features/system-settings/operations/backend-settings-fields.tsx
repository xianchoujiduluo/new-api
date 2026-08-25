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
  return (
    <FormField
      control={props.control}
      name='downloadProxy'
      render={({ field }) => (
        <FormItem className='lg:col-span-2'>
          <FormLabel>{props.t('Proxy Address')}</FormLabel>
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
