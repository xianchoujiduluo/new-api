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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { getBackendUpdateStatus } from '../api'

// The launcher health-checks a candidate before promoting it and removes the
// `pending` link either way, so the backend answering again with no pending
// release is the definitive "the switch is over" signal.
const POLL_INTERVAL_MS = 2_000
const MAX_WAIT_MS = 5 * 60 * 1_000

export function useBackendRestartWatch(onSettled: () => void) {
  const { t } = useTranslation()
  const [restarting, setRestarting] = useState(false)
  const [seconds, setSeconds] = useState(0)

  useEffect(() => {
    if (!restarting) return

    let cancelled = false
    let timer: ReturnType<typeof setTimeout> | undefined
    const startedAt = Date.now()

    const poll = async () => {
      if (cancelled) return
      try {
        const response = await getBackendUpdateStatus()
        const status = response.success ? response.data : undefined
        if (cancelled) return
        if (status && !status.pending_version) {
          setRestarting(false)
          toast.success(
            t('Backend restarted on {{version}}', {
              version: status.current_version || t('unknown version'),
            })
          )
          onSettled()
          return
        }
      } catch {
        // Connection refused while the process is down: keep waiting.
      }
      if (cancelled) return
      if (Date.now() - startedAt > MAX_WAIT_MS) {
        setRestarting(false)
        toast.warning(
          t(
            'The backend did not report back in time. Reload the page to check the current version.'
          )
        )
        onSettled()
        return
      }
      timer = setTimeout(poll, POLL_INTERVAL_MS)
    }

    timer = setTimeout(poll, POLL_INTERVAL_MS)
    const counter = setInterval(() => setSeconds((value) => value + 1), 1_000)

    return () => {
      cancelled = true
      if (timer) clearTimeout(timer)
      clearInterval(counter)
    }
  }, [restarting, t, onSettled])

  return {
    restarting,
    seconds,
    begin: () => {
      setSeconds(0)
      setRestarting(true)
    },
  }
}
