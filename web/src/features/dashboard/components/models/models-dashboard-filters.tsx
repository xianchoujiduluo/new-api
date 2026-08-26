/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import { Calendar, ChevronDown, ListFilter, RotateCcw } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { MultiSelect } from '@/components/multi-select'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Label } from '@/components/ui/label'
import { DASHBOARD_FILTER_TIME_RANGE_PRESETS } from '@/features/dashboard/constants'
import {
  buildDefaultDashboardFilters,
  detectDashboardPresetKey,
  getDashboardPresetDateRange,
} from '@/features/dashboard/lib'
import type {
  DashboardChartPreferences,
  DashboardFilterOptions,
  DashboardFilters,
} from '@/features/dashboard/types'
import { CompactDateTimeRangePicker } from '@/features/usage-logs/components/compact-date-time-range-picker'
import { cn } from '@/lib/utils'

interface ModelsDashboardFiltersProps {
  preferences: DashboardChartPreferences
  filters: DashboardFilters
  options: DashboardFilterOptions
  onChange: (filters: DashboardFilters) => void
}

function granularityForRange(days: number) {
  if (days <= 1) return 'hour' as const
  if (days >= 29) return 'week' as const
  return 'day' as const
}

export function ModelsDashboardFilters(props: ModelsDashboardFiltersProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(true)
  const activeRange = detectDashboardPresetKey(props.filters)
  const activePreset = DASHBOARD_FILTER_TIME_RANGE_PRESETS.find(
    (preset) => preset.key === activeRange
  )

  const updateRange = (
    preset: (typeof DASHBOARD_FILTER_TIME_RANGE_PRESETS)[number]
  ) => {
    const range = getDashboardPresetDateRange(preset)
    props.onChange({
      ...props.filters,
      start_timestamp: range.start,
      end_timestamp: range.end,
      time_granularity: granularityForRange(preset.days),
    })
  }

  const resetFilters = () => {
    props.onChange(buildDefaultDashboardFilters(props.preferences))
  }

  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className='border-border/70 bg-card/40 overflow-hidden rounded-lg border'
    >
      <div className='flex min-h-10 items-center gap-2 px-3 py-2'>
        <div className='flex min-w-0 items-center gap-2'>
          <ListFilter
            className='text-muted-foreground size-3.5 shrink-0'
            aria-hidden='true'
          />
          <span className='text-sm font-medium'>{t('Filters')}</span>
          {!open && (
            <span className='text-muted-foreground truncate text-xs'>
              {activePreset ? t(activePreset.label) : t('Custom Time Range')}
            </span>
          )}
        </div>
        <div className='ml-auto flex shrink-0 items-center gap-1'>
          <Button
            type='button'
            variant='ghost'
            size='icon'
            onClick={resetFilters}
            aria-label={t('Reset')}
            className='text-muted-foreground size-7'
          >
            <RotateCcw className='size-3.5' />
          </Button>
          <CollapsibleTrigger
            render={
              <Button
                type='button'
                variant='ghost'
                size='icon'
                aria-label={open ? t('Collapse') : t('Expand')}
                title={open ? t('Collapse') : t('Expand')}
                className='text-muted-foreground size-7'
              />
            }
          >
            <ChevronDown
              className={cn(
                'size-4 transition-transform duration-200',
                open && 'rotate-180'
              )}
            />
          </CollapsibleTrigger>
        </div>
      </div>

      <CollapsibleContent className='data-closed:animate-out data-closed:fade-out-0 data-closed:slide-out-to-top-1 data-open:animate-in data-open:fade-in-0 data-open:slide-in-from-top-1 border-t outline-none'>
        <div className='flex flex-col gap-3 p-3'>
          <div className='flex flex-wrap items-center gap-2'>
            <div className='text-muted-foreground flex items-center gap-1.5 text-xs font-medium'>
              <Calendar className='size-3.5' aria-hidden='true' />
              {t('Time Range')}
            </div>
            <div className='flex flex-wrap gap-1.5'>
              {DASHBOARD_FILTER_TIME_RANGE_PRESETS.map((preset) => (
                <Button
                  key={preset.key}
                  type='button'
                  size='sm'
                  variant={activeRange === preset.key ? 'default' : 'outline'}
                  onClick={() => updateRange(preset)}
                  className='h-7 px-2.5 text-xs'
                >
                  {t(preset.label)}
                </Button>
              ))}
            </div>
          </div>

          <div className='grid gap-3 lg:grid-cols-[minmax(0,1.35fr)_minmax(0,1fr)_minmax(0,1fr)]'>
            <div className='grid min-w-0 gap-1.5'>
              <Label className='text-muted-foreground text-xs'>
                {t('Time Range')}
              </Label>
              <CompactDateTimeRangePicker
                start={props.filters.start_timestamp}
                end={props.filters.end_timestamp}
                showPresets={false}
                onChange={(range) =>
                  props.onChange({
                    ...props.filters,
                    start_timestamp: range.start,
                    end_timestamp: range.end,
                  })
                }
              />
            </div>
            <div className='grid gap-1.5'>
              <Label className='text-muted-foreground text-xs'>
                {t('Channels')}
              </Label>
              <MultiSelect
                options={props.options.channels}
                selected={(props.filters.channel_ids ?? []).map(String)}
                onChange={(values) =>
                  props.onChange({
                    ...props.filters,
                    channel_ids: values
                      .map(Number)
                      .filter((value) => value > 0),
                  })
                }
                placeholder={t('All Channels')}
                emptyText={t('No channels found')}
                allowMultipleToggle
                clearable
                searchInPopup
              />
            </div>
            <div className='grid gap-1.5'>
              <Label className='text-muted-foreground text-xs'>
                {t('Models')}
              </Label>
              <MultiSelect
                options={props.options.models}
                selected={props.filters.model_names ?? []}
                onChange={(values) =>
                  props.onChange({ ...props.filters, model_names: values })
                }
                placeholder={t('All Models')}
                emptyText={t('No available models')}
                allowMultipleToggle
                clearable
                searchInPopup
              />
            </div>
          </div>
        </div>
      </CollapsibleContent>
    </Collapsible>
  )
}
