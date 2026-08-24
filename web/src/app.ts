import type { Catalog, Device, Incident, Service } from './types'

function node<K extends keyof HTMLElementTagNameMap>(
  tag: K,
  className?: string,
  text?: string,
): HTMLElementTagNameMap[K] {
  const element = document.createElement(tag)
  if (className) element.className = className
  if (text !== undefined) element.textContent = text
  return element
}

function relativeTime(value: string): string {
  const seconds = Math.max(0, Math.floor((Date.now() - new Date(value).getTime()) / 1000))
  if (seconds < 10) return '刚刚'
  if (seconds < 60) return String(seconds) + ' 秒前'
  if (seconds < 3600) return String(Math.floor(seconds / 60)) + ' 分钟前'
  return String(Math.floor(seconds / 3600)) + ' 小时前'
}

function stateLabel(state: Device['state'] | Service['state']): string {
  return {
    candidate: '发现中',
    established: '已建立',
    online: '在线',
    offline: '离线',
  }[state]
}

function serviceCard(service: Service, devices: Map<string, Device>): HTMLElement {
  const link = node('a', 'card service state-' + service.state)
  link.href = service.url
  link.target = '_blank'
  link.rel = 'noopener noreferrer'
  const head = node('div', 'card-head')
  head.append(node('strong', '', service.name), node('span', 'state', stateLabel(service.state)))
  const device = devices.get(service.deviceId)
  const path = service.path === '/' ? '' : service.path
  link.append(
    head,
    node('span', 'meta', (device?.name || device?.ipv4 || '未知设备') + ' · ' + String(service.port) + path),
    node('span', 'address', service.url),
  )
  return link
}

function deviceCard(device: Device, services: Service[]): HTMLElement {
  const article = node('article', 'card device state-' + device.state)
  const head = node('div', 'card-head')
  head.append(node('strong', '', device.name), node('span', 'state', stateLabel(device.state)))
  article.append(
    head,
    node('span', 'meta', device.domain + ' · ' + device.interface + ' · ' + device.mac),
    node('span', 'address', device.ipv4 + ' · ' + String(services.length) + ' 个服务'),
  )
  return article
}

function incidentCard(incident: Incident): HTMLElement {
  const article = node('article', 'incident ' + (incident.resolvedAt ? 'resolved' : 'active'))
  const head = node('div', 'card-head')
  head.append(node('strong', '', incident.name), node('span', 'state', incident.resolvedAt ? '已恢复' : '故障中'))
  let period = new Date(incident.startedAt).toLocaleString('zh-CN')
  if (incident.resolvedAt) period += ' → ' + new Date(incident.resolvedAt).toLocaleString('zh-CN')
  article.append(head, node('p', '', incident.reason || '连接失败'), node('span', 'meta', period))
  return article
}

function section(title: string, count: number): { section: HTMLElement; body: HTMLElement } {
  const container = node('section', 'panel')
  const heading = node('div', 'section-title')
  heading.append(node('h2', '', title), node('span', '', String(count) + ' 项'))
  const body = node('div', 'grid')
  container.append(heading, body)
  return { section: container, body }
}

export function renderCatalog(root: HTMLElement, catalog: Catalog, query = ''): void {
  root.replaceChildren()
  const search = query.trim().toLowerCase()
  const devicesByID = new Map(catalog.devices.map((device) => [device.id, device]))
  const matches = (text: string) => !search || text.toLowerCase().includes(search)
  const services = catalog.services.filter((service) => {
    const device = devicesByID.get(service.deviceId)
    return matches(service.name + ' ' + service.url + ' ' + (device?.name || '') + ' ' + (device?.ipv4 || ''))
  })
  const devices = catalog.devices.filter((device) => matches(device.name + ' ' + device.ipv4 + ' ' + device.mac))

  const summary = node('section', 'summary')
  const title = node('div')
  const discoveryDate = new Date(catalog.lastDiscoveryAt)
  const discoveryText = discoveryDate.getUTCFullYear() >= 2000
    ? '自动发现于 ' + relativeTime(catalog.lastDiscoveryAt)
    : '等待首次自动发现'
  title.append(node('span', 'eyebrow', 'AUTO-NAV'), node('h1', '', '局域网服务'), node('p', '', discoveryText))
  const metrics = node('div', 'metrics')
  metrics.append(
    metric(String(catalog.devices.filter((item) => item.state === 'online').length), '在线设备'),
    metric(String(catalog.services.filter((item) => item.state === 'online').length), '可用服务'),
    metric(String(catalog.incidents.filter((item) => !item.resolvedAt).length), '当前故障'),
  )
  summary.append(title, metrics)
  root.append(summary)
  if (catalog.discoveryError) root.append(node('p', 'load-error', '自动发现暂不可用：' + catalog.discoveryError))

  const routerServices = services.filter((service) => devicesByID.get(service.deviceId)?.router)
  if (routerServices.length) {
    const group = section('路由器', routerServices.length)
    routerServices.forEach((service) => group.body.append(serviceCard(service, devicesByID)))
    root.append(group.section)
  }

  const otherServices = services.filter((service) => !devicesByID.get(service.deviceId)?.router)
  const serviceGroup = section('自动发现的服务', otherServices.length)
  if (otherServices.length) otherServices.forEach((service) => serviceGroup.body.append(serviceCard(service, devicesByID)))
  else serviceGroup.body.append(node('p', 'empty', '正在发现局域网中的 Web 服务。'))
  root.append(serviceGroup.section)

  const deviceGroup = section('局域网客户端', devices.length)
  devices.forEach((device) => deviceGroup.body.append(deviceCard(device, services.filter((service) => service.deviceId === device.id))))
  root.append(deviceGroup.section)

  const recent = catalog.incidents.slice(0, 10)
  const incidentGroup = section('最近故障', recent.length)
  if (recent.length) recent.forEach((incident) => incidentGroup.body.append(incidentCard(incident)))
  else incidentGroup.body.append(node('p', 'empty', '暂无已确认故障。'))
  root.append(incidentGroup.section)

  const footer = node('footer')
  footer.textContent = 'MeowDeck ' + catalog.version + ' · IPv4 LAN · 本次启动持久化 ' + String(catalog.persistenceWritesSinceBoot) + ' 次'
  root.append(footer)
}

function metric(value: string, label: string): HTMLElement {
  const item = node('div', 'metric')
  item.append(node('strong', '', value), node('span', '', label))
  return item
}
