import type { ServiceStatus, StatusResponse } from './types'

const states = (state: ServiceStatus['state'], count = 16) =>
  Array.from({ length: count }, (_, index) => ({
    at: new Date(Date.now() - (count - index) * 30_000).toISOString(),
    state,
    latencyMs: state === 'online' ? 8 + (index % 7) : undefined,
  }))

export const demoStatus: StatusResponse = {
  version: 'dev',
  generatedAt: new Date().toISOString(),
  hostname: 'meow.lan',
  intervalSeconds: 30,
  services: [
    {
      id: 'router',
      slug: 'router',
      name: 'GL.iNet 管理后台',
      description: 'GL-MT3600BE · 端口 80',
      category: 'network',
      icon: 'router',
      url: '/router',
      state: 'online',
      latencyMs: 1,
      lastChecked: new Date().toISOString(),
      editable: false,
      history: states('online'),
    },
    {
      id: 'luci',
      slug: 'luci',
      name: 'LuCI 高级管理',
      description: 'OpenWrt · 端口 8080',
      category: 'network',
      icon: 'terminal',
      url: '/luci',
      state: 'online',
      latencyMs: 2,
      lastChecked: new Date().toISOString(),
      editable: false,
      history: states('online'),
    },
  ],
}
