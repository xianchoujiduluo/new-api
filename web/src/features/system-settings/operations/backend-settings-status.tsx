type VersionValueProps = {
  label: string
  value?: string
}

export function VersionValue(props: VersionValueProps) {
  return (
    <div className='rounded-lg border p-4'>
      <div className='text-muted-foreground text-sm'>{props.label}</div>
      <div className='break-all text-lg font-semibold'>
        {props.value || '-'}
      </div>
    </div>
  )
}
