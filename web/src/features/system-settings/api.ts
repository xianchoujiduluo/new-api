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
import { api } from '@/lib/api'

import type {
  ConfirmPaymentComplianceResponse,
  BackendCheckResult,
  BackendDownloadJob,
  FrontendDownloadJob,
  BackendUpdateResponse,
  BackendUpdateResult,
  BackendUpdateStatus,
  FetchUpstreamRatiosRequest,
  LogCleanupTask,
  SystemOptionsResponse,
  SystemTaskListResponse,
  SystemTaskResponse,
  UpdateOptionRequest,
  UpdateOptionResponse,
  UpstreamChannelsResponse,
  UpstreamRatiosResponse,
} from './types'

export async function getSystemOptions() {
  const res = await api.get<SystemOptionsResponse>('/api/option/')
  return res.data
}

export async function updateSystemOption(request: UpdateOptionRequest) {
  const res = await api.put<UpdateOptionResponse>('/api/option/', request)
  return res.data
}

export async function startFrontendDownload() {
  const res = await api.post<BackendUpdateResponse<FrontendDownloadJob>>(
    '/api/frontend/download',
    undefined,
    { skipBusinessError: true }
  )
  return res.data
}

/** Polls frontend staging progress; falls back to the on-disk staged release. */
export async function getFrontendDownloadJob() {
  const res = await api.get<BackendUpdateResponse<FrontendDownloadJob>>(
    '/api/frontend/download',
    { skipBusinessError: true }
  )
  return res.data
}

/** Swaps the staged frontend into place; a directory rename, so it is instant. */
export async function activateFrontend() {
  const res = await api.post<BackendUpdateResponse<null>>(
    '/api/frontend/activate',
    undefined,
    { skipBusinessError: true }
  )
  return res.data
}

export async function discardFrontendDownload() {
  const res = await api.delete<BackendUpdateResponse<{ discarded: boolean }>>(
    '/api/frontend/staging',
    { skipBusinessError: true }
  )
  return res.data
}

export async function getBackendUpdateStatus() {
  const res = await api.get<BackendUpdateResponse<BackendUpdateStatus>>(
    '/api/backend-update/status',
    { skipBusinessError: true }
  )
  return res.data
}

export async function checkBackendUpdate() {
  const res = await api.post<BackendUpdateResponse<BackendCheckResult>>(
    '/api/backend-update/check',
    undefined,
    { skipBusinessError: true }
  )
  return res.data
}

export async function startBackendDownload() {
  const res = await api.post<BackendUpdateResponse<BackendDownloadJob>>(
    '/api/backend-update/download',
    undefined,
    { skipBusinessError: true }
  )
  return res.data
}

/**
 * Polls download progress. `skipBusinessError` keeps a `409` (already running)
 * reporting the current job instead of throwing.
 */
export async function getBackendDownloadJob() {
  const res = await api.get<BackendUpdateResponse<BackendDownloadJob>>(
    '/api/backend-update/download',
    { skipBusinessError: true }
  )
  return res.data
}

/** Discards a staged release without restarting. */
export async function discardBackendDownload() {
  const res = await api.delete<BackendUpdateResponse<{ discarded: boolean }>>(
    '/api/backend-update/pending',
    { skipBusinessError: true }
  )
  return res.data
}

/** Hands control to the launcher, which activates the staged release. */
export async function restartBackend() {
  const res = await api.post<BackendUpdateResponse<null>>(
    '/api/backend-update/restart',
    undefined,
    { skipBusinessError: true }
  )
  return res.data
}

/** Stages the previous release for activation; a separate restart is required. */
export async function rollbackBackendUpdate() {
  const res = await api.post<BackendUpdateResponse<BackendUpdateResult>>(
    '/api/backend-update/rollback',
    undefined,
    { skipBusinessError: true }
  )
  return res.data
}

export async function confirmPaymentCompliance() {
  const res = await api.post<ConfirmPaymentComplianceResponse>(
    '/api/option/payment_compliance',
    { confirmed: true }
  )
  return res.data
}

export async function startLogCleanupTask(targetTimestamp: number) {
  const res = await api.post<SystemTaskResponse<LogCleanupTask>>(
    '/api/system-task/log-cleanup',
    null,
    {
      params: { target_timestamp: targetTimestamp },
    }
  )
  return res.data
}

export async function getCurrentLogCleanupTask() {
  const res = await api.get<SystemTaskResponse<LogCleanupTask | null>>(
    '/api/system-task/current',
    {
      params: { type: 'log_cleanup' },
    }
  )
  return res.data
}

export async function getSystemTask(taskId: string) {
  const res = await api.get<SystemTaskResponse<LogCleanupTask>>(
    `/api/system-task/${taskId}`
  )
  return res.data
}

export async function listSystemTasks(limit = 20) {
  const res = await api.get<SystemTaskListResponse>('/api/system-task/list', {
    params: { limit },
  })
  return res.data
}

export async function resetModelRatios() {
  const res = await api.post<UpdateOptionResponse>(
    '/api/option/rest_model_ratio'
  )
  return res.data
}

export async function getUpstreamChannels() {
  const res = await api.get<UpstreamChannelsResponse>(
    '/api/ratio_sync/channels'
  )
  return res.data
}

export async function fetchUpstreamRatios(request: FetchUpstreamRatiosRequest) {
  const res = await api.post<UpstreamRatiosResponse>(
    '/api/ratio_sync/fetch',
    request
  )
  return res.data
}
