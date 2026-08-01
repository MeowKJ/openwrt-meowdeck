export type ServiceCategory = 'network' | 'automation' | 'smart-home' | 'device'
export type ServiceState = 'online' | 'degraded' | 'offline' | 'disabled' | 'checking'
export type ProbeType = 'http' | 'tcp' | 'ping' | 'process'

export interface HeartbeatPoint {
  at: string
  state: ServiceState
  latencyMs?: number
}

export interface ServiceStatus {
  id: string
  slug: string
  subdomain?: string
  name: string
  description: string
  category: ServiceCategory
  icon: string
  url?: string
  state: ServiceState
  latencyMs?: number
  lastChecked?: string
  message?: string
  editable: boolean
  history: HeartbeatPoint[]
}

export interface ServiceInput {
  id: string
  slug: string
  subdomain?: string
  name: string
  description: string
  category: ServiceCategory
  icon: string
  url?: string
  proxy?: boolean
  probe: {
    type: ProbeType
    target: string
  }
}

export interface StatusResponse {
  version: string
  generatedAt: string
  hostname: string
  intervalSeconds: number
  services: ServiceStatus[]
}
