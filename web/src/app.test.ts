import { renderCatalog } from './app'
import type { Catalog } from './types'
import { expect, test } from 'vitest'

const sample: Catalog = {
  version: 'test',
  generatedAt: '2026-08-17T08:00:00Z',
  ipv4Only: true,
  lastDiscoveryAt: new Date().toISOString(),
  persistenceWritesSinceBoot: 1,
  devices: [{
    id: 'router', domain: 'lan', interface: 'br-lan', mac: 'aa:bb:cc:dd:ee:ff',
    ipv4: '192.168.8.1', name: '<b>Router</b>', router: true, state: 'online',
    firstSeen: '2026-08-17T08:00:00Z', lastSeen: '2026-08-17T08:00:00Z',
  }],
  services: [{
    id: 'luci', deviceId: 'router', scheme: 'http', port: 80, path: '/cgi-bin/luci/',
    type: 'luci', name: '<img src=x onerror=alert(1)>', url: 'http://192.168.8.1/cgi-bin/luci/',
    state: 'online', firstSeen: '2026-08-17T08:00:00Z', lastSeen: '2026-08-17T08:00:00Z',
  }],
  incidents: [],
}

test('renders untrusted discovery names as text', () => {
  const root = document.createElement('div')
  renderCatalog(root, sample)
  expect(root.querySelector('img')).toBeNull()
  expect(root.textContent).toContain('<img src=x onerror=alert(1)>')
})

test('filters by device address', () => {
  const root = document.createElement('div')
  renderCatalog(root, sample, '192.168.8.1')
  expect(root.textContent).toContain('<b>Router</b>')
})

test('does not render Go zero time as an enormous relative duration', () => {
  const root = document.createElement('div')
  renderCatalog(root, { ...sample, lastDiscoveryAt: '0001-01-01T00:00:00Z' })
  expect(root.textContent).toContain('等待首次自动发现')
  expect(root.textContent).not.toContain('小时前')
})
