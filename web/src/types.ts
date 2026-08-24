export type DeviceState = 'candidate' | 'online' | 'offline'
export type ServiceState = 'candidate' | 'established' | 'online' | 'offline'

export interface Device {
  id: string
  domain: string
  interface: string
  mac: string
  ipv4: string
  name: string
  router?: boolean
  state: DeviceState
  firstSeen: string
  lastSeen: string
}

export interface Service {
  id: string
  deviceId: string
  scheme: string
  port: number
  path: string
  type: string
  name: string
  url: string
  state: ServiceState
  firstSeen: string
  lastSeen: string
  latencyMs?: number
}

export interface Incident {
  id: string
  subjectId: string
  subjectType: 'device' | 'service'
  name: string
  reason: string
  startedAt: string
  resolvedAt?: string
}

export interface Catalog {
  version: string
  generatedAt: string
  ipv4Only: boolean
  lastDiscoveryAt: string
  discoveryError?: string
  persistenceWritesSinceBoot: number
  devices: Device[]
  services: Service[]
  incidents: Incident[]
}
