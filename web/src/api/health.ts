export interface HealthResponse {
  status: string
  version: string
  serverTime: number // Unix 毫秒
}

export async function fetchHealth(signal?: AbortSignal): Promise<HealthResponse> {
  const res = await fetch('/api/healthz', { signal })
  if (!res.ok) {
    throw new Error(`HTTP ${res.status}`)
  }
  return (await res.json()) as HealthResponse
}
