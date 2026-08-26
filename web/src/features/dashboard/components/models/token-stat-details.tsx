/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import { BrainCircuit, Database, LogIn, LogOut, Percent } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Skeleton } from '@/components/ui/skeleton'
import { formatCompactNumber, formatNumber } from '@/lib/format'

interface TokenStatDetailsProps {
  header: ReactNode
  totalTokens: number
  inputTokens: number
  cachedTokens: number
  reasoningTokens: number
  locale: Intl.LocalesArgument
  loading: boolean
  error: boolean
}

const MAX_INLINE_CHARS = 9

function formatTokenCount(value: number, locale: Intl.LocalesArgument) {
  const fullValue = formatNumber(value, locale)
  return {
    displayValue:
      fullValue.length > MAX_INLINE_CHARS
        ? formatCompactNumber(value, locale)
        : fullValue,
    fullValue,
  }
}

export function TokenStatDetails(props: TokenStatDetailsProps) {
  const { t } = useTranslation()
  if (props.loading) {
    return (
      <div className='grid gap-3 sm:grid-cols-[minmax(0,0.8fr)_minmax(0,1.4fr)] sm:items-center sm:gap-4'>
        <div className='flex min-w-0 flex-col justify-center gap-1.5'>
          {props.header}
          <Skeleton className='h-7 w-24 sm:h-8' />
        </div>
        <div className='grid grid-cols-2 gap-x-4 gap-y-2'>
          {Array.from({ length: 5 }, (_, index) => (
            <Skeleton key={index} className='h-8 w-full' />
          ))}
        </div>
      </div>
    )
  }

  if (props.error) {
    return (
      <div className='grid gap-3 sm:grid-cols-[minmax(0,0.8fr)_minmax(0,1.4fr)] sm:items-center sm:gap-4'>
        <div className='flex min-w-0 flex-col justify-center gap-1.5'>
          {props.header}
          <div className='text-muted-foreground font-mono text-xl font-bold'>
            --
          </div>
        </div>
      </div>
    )
  }

  const total = formatTokenCount(props.totalTokens, props.locale)
  const outputTokens = Math.max(props.totalTokens - props.inputTokens, 0)
  const cacheHitRate =
    props.inputTokens > 0 ? (props.cachedTokens / props.inputTokens) * 100 : 0
  const details = [
    { label: t('Input Tokens'), value: props.inputTokens, icon: LogIn },
    { label: t('Output Tokens'), value: outputTokens, icon: LogOut },
    { label: t('Cached input'), value: props.cachedTokens, icon: Database },
    { label: t('Reasoning'), value: props.reasoningTokens, icon: BrainCircuit },
  ]

  return (
    <div className='grid gap-3 sm:grid-cols-[minmax(0,0.8fr)_minmax(0,1.4fr)] sm:items-center sm:gap-4'>
      <div className='flex min-w-0 flex-col justify-center gap-1.5'>
        {props.header}
        <div
          className='font-mono text-lg leading-tight font-bold tabular-nums sm:text-2xl'
          title={total.fullValue}
        >
          {total.displayValue}
        </div>
      </div>
      <div className='grid grid-cols-2 gap-x-4 gap-y-2'>
        {details.map((detail) => {
          const value = formatTokenCount(detail.value, props.locale)
          const Icon = detail.icon
          return (
            <div key={detail.label} className='min-w-0'>
              <div className='text-muted-foreground flex items-center gap-1 text-[11px]'>
                <Icon className='size-3 shrink-0' aria-hidden='true' />
                <span className='truncate'>{detail.label}</span>
              </div>
              <div
                className='mt-0.5 truncate font-mono text-sm font-semibold tabular-nums'
                title={value.fullValue}
              >
                {value.displayValue}
              </div>
            </div>
          )
        })}
        <div className='min-w-0'>
          <div className='text-muted-foreground flex items-center gap-1 text-[11px]'>
            <Percent className='size-3 shrink-0' aria-hidden='true' />
            <span className='truncate'>{t('Cache Hit Rate')}</span>
          </div>
          <div className='mt-0.5 font-mono text-sm font-semibold tabular-nums'>
            {cacheHitRate.toFixed(2)}%
          </div>
        </div>
      </div>
    </div>
  )
}
