import { demoStatus } from './mock'
import type { ServiceInput, StatusResponse } from './types'

const REQUEST_TIMEOUT_MS = 4_000

export async function fetchStatus(signal?: AbortSignal): Promise<{ data: StatusResponse; demo: boolean }> {
  const timeout = AbortSignal.timeout(REQUEST_TIMEOUT_MS)
  const combinedSignal = signal ? AbortSignal.any([signal, timeout]) : timeout

  try {
    const response = await fetch('/api/v1/status', {
      headers: { Accept: 'application/json' },
      cache: 'no-store',
      signal: combinedSignal,
    })
    if (!response.ok) throw new Error(`status API returned ${response.status}`)
    return { data: (await response.json()) as StatusResponse, demo: false }
  } catch (error) {
    if (signal?.aborted) throw error
    return {
      data: { ...demoStatus, generatedAt: new Date().toISOString() },
      demo: true,
    }
  }
}

async function readAPIError(response: Response) {
  try {
    const body = await response.json() as { error?: string }
    return body.error || `请求失败（${response.status}）`
  } catch {
    return `请求失败（${response.status}）`
  }
}

export async function addService(service: ServiceInput): Promise<void> {
  const response = await fetch('/api/v1/services', {
    method: 'POST',
    headers: {
      Accept: 'application/json',
      'Content-Type': 'application/json',
      'X-MeowDeck-Edit': '1',
    },
    body: JSON.stringify(service),
  })
  if (!response.ok) throw new Error(await readAPIError(response))
}

export async function deleteService(id: string): Promise<void> {
  const response = await fetch(`/api/v1/services/${encodeURIComponent(id)}`, {
    method: 'DELETE',
    headers: { 'X-MeowDeck-Edit': '1' },
  })
  if (!response.ok) throw new Error(await readAPIError(response))
}
