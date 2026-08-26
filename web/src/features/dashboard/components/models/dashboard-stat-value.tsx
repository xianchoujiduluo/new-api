/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import { Skeleton } from '@/components/ui/skeleton'

interface DashboardStatValueProps {
  value: string
  fullValue: string
  description: string
  loading: boolean
  error: boolean
}

export function DashboardStatValue(props: DashboardStatValueProps) {
  if (props.loading) {
    return (
      <div className='mt-1 flex flex-col gap-1 sm:mt-2 sm:gap-1.5'>
        <Skeleton className='h-5 w-16 sm:h-7 sm:w-20' />
        <Skeleton className='hidden h-3.5 w-28 md:block' />
      </div>
    )
  }

  return (
    <>
      <div
        className='text-foreground mt-1 max-w-full truncate font-mono text-base leading-tight font-bold tracking-tight tabular-nums sm:mt-2 sm:text-2xl sm:leading-normal'
        title={props.error ? undefined : props.fullValue}
      >
        {props.error ? '--' : props.value}
      </div>
      <div className='text-muted-foreground/60 mt-1 hidden text-xs md:block'>
        {props.description}
      </div>
    </>
  )
}
