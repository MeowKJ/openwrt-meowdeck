import { type FormEvent, useCallback, useEffect, useMemo, useState } from 'react'
import { motion } from 'motion/react'
import {
  Activity,
  Cat,
  Cpu,
  ExternalLink,
  Globe2,
  HeartPulse,
  HousePlug,
  Plus,
  RadioTower,
  RefreshCw,
  Router,
  ShieldCheck,
  Sprout,
  TerminalSquare,
  Trash2,
  Wifi,
  WifiOff,
  X,
} from 'lucide-react'
import { addService, deleteService, fetchStatus } from './api'
import { demoStatus } from './mock'
import type { ProbeType, ServiceCategory, ServiceInput, ServiceState, ServiceStatus, StatusResponse } from './types'

const POLL_INTERVAL_MS = 30_000

const categories: Array<{ id: ServiceCategory; label: string; hint: string }> = [
  { id: 'network', label: '核心网络', hint: '路由、系统与网络服务' },
  { id: 'automation', label: '自动化', hint: '后台任务与无人值守服务' },
  { id: 'smart-home', label: '智能家居', hint: '家庭设备与控制中心' },
  { id: 'device', label: '设备', hint: '本地计算节点' },
]

const stateLabels: Record<ServiceState, string> = {
  online: '在线', degraded: '需关注', offline: '离线', disabled: '待接入', checking: '检查中',
}

const iconMap = {
  router: Router, terminal: TerminalSquare, shield: ShieldCheck,
  sprout: Sprout, house: HousePlug, cpu: Cpu, globe: Globe2, pulse: HeartPulse,
}

const probeLabels: Record<ProbeType, string> = {
  http: 'HTTP 页面', tcp: 'TCP 端口', ping: 'Ping 主机', process: '本机进程',
}

function relativeTime(value?: string) {
  if (!value) return '尚未检查'
  const seconds = Math.max(0, Math.round((Date.now() - new Date(value).getTime()) / 1_000))
  if (seconds < 10) return '刚刚'
  if (seconds < 60) return `${seconds} 秒前`
  return `${Math.floor(seconds / 60)} 分钟前`
}

function ServiceCard({
  service,
  hostname,
  index,
  deleting,
  onDelete,
}: {
  service: ServiceStatus
  hostname: string
  index: number
  deleting: boolean
  onDelete: (service: ServiceStatus) => void
}) {
  const Icon = iconMap[service.icon as keyof typeof iconMap] ?? Activity
  const href = service.url
  const localAddress = service.subdomain
    ? `${service.subdomain}.${hostname}`
    : `${hostname}/${service.slug}`
  const healthText = service.state === 'online' && service.latencyMs !== undefined
    ? `${service.latencyMs} ms`
    : service.state === 'offline'
      ? '连接失败'
      : service.message ?? '—'

  return (
    <motion.article
      className={`service-card state-${service.state}`}
      initial={{ opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.35, delay: index * 0.045 }}
      whileHover={{ y: -4 }}
    >
      <div className="card-glow" aria-hidden="true" />
      <div className="service-card__head">
        <div className="service-icon" aria-hidden="true"><Icon size={20} strokeWidth={1.8} /></div>
        <div className="service-title">
          <h3>{service.name}</h3>
          <p>{service.description}</p>
        </div>
        <span className={`state-pill state-pill--${service.state}`}>
          <span className="state-dot" aria-hidden="true" />
          {stateLabels[service.state]}
        </span>
      </div>

      <div className="local-address" title={localAddress}><Globe2 size={13} /> {localAddress}</div>

      <div className="heartbeat-row">
        <div className="heartbeat-strip" aria-label={`最近 ${service.history.length} 次心跳`}>
          {service.history.slice(-18).map((point, pointIndex) => (
            <span
              className={`heartbeat-point heartbeat-point--${point.state}`}
              key={`${point.at}-${pointIndex}`}
              title={`${new Date(point.at).toLocaleTimeString()} · ${stateLabels[point.state]}`}
            />
          ))}
        </div>
        <span className="latency" title={service.message}>{healthText}</span>
      </div>

      <div className="service-card__foot">
        <span>检查于 {relativeTime(service.lastChecked)}</span>
        <div className="card-actions">
          {service.editable && (
            <button
              className="delete-button"
              type="button"
              disabled={deleting}
              onClick={() => onDelete(service)}
              aria-label={`删除 ${service.name}`}
              title="删除服务"
            >
              <Trash2 size={14} />
            </button>
          )}
          {href ? (
            <a href={href} target="_blank" rel="noreferrer" aria-label={`打开 ${service.name}`}>
              打开后台 <ExternalLink size={14} aria-hidden="true" />
            </a>
          ) : (
            <span className="disabled-link">仅状态</span>
          )}
        </div>
      </div>
    </motion.article>
  )
}

function AddServicePanel({
  hostname,
  onClose,
  onAdded,
}: {
  hostname: string
  onClose: () => void
  onAdded: () => Promise<void>
}) {
  const [name, setName] = useState('')
  const [slug, setSlug] = useState('')
  const [description, setDescription] = useState('')
  const [category, setCategory] = useState<ServiceCategory>('device')
  const [icon, setIcon] = useState('cpu')
  const [url, setURL] = useState('')
  const [probeType, setProbeType] = useState<ProbeType>('http')
  const [probeTarget, setProbeTarget] = useState('')
  const [subdomain, setSubdomain] = useState('')
  const [proxy, setProxy] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  const pathPreview = `http://${hostname}/${slug || '项目名'}`
  const subdomainPreview = subdomain ? `http://${subdomain}.${hostname}` : ''
  const probePlaceholder = {
    http: 'http://192.168.8.178:8080/health',
    tcp: '192.168.8.178:8080',
    ping: '192.168.8.178',
    process: 'tailscaled',
  }[probeType]

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setSaving(true)
    setError('')
    const service: ServiceInput = {
      id: slug.trim(),
      slug: slug.trim(),
      name: name.trim(),
      description: description.trim() || '自定义本地服务',
      category,
      icon,
      url: url.trim() || undefined,
      subdomain: subdomain.trim() || undefined,
      proxy: Boolean(subdomain && proxy),
      probe: {
        type: probeType,
        target: probeTarget.trim() || (probeType === 'http' ? url.trim() : ''),
      },
    }
    try {
      await addService(service)
      await onAdded()
      onClose()
    } catch (submitError) {
      setError(submitError instanceof Error ? submitError.message : '添加失败')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="panel-backdrop" role="presentation" onMouseDown={(event) => {
      if (event.target === event.currentTarget) onClose()
    }}>
      <motion.section
        className="add-panel"
        role="dialog"
        aria-modal="true"
        aria-labelledby="add-service-title"
        initial={{ opacity: 0, x: 32 }}
        animate={{ opacity: 1, x: 0 }}
      >
        <div className="panel-heading">
          <div>
            <span className="eyebrow">NEW SERVICE</span>
            <h2 id="add-service-title">添加本地服务</h2>
            <p>创建入口并选择一种轻量心跳检查。</p>
          </div>
          <button className="icon-button" type="button" onClick={onClose} aria-label="关闭添加面板"><X size={18} /></button>
        </div>

        <form className="service-form" onSubmit={(event) => void submit(event)}>
          <div className="form-grid">
            <label>
              <span>显示名称</span>
              <input required value={name} onChange={(event) => setName(event.target.value)} placeholder="Home Assistant" />
            </label>
            <label>
              <span>项目标识</span>
              <input
                required
                pattern="[a-z0-9](?:(?:[a-z0-9]|-)*[a-z0-9])?"
                maxLength={63}
                value={slug}
                onChange={(event) => setSlug(event.target.value.toLowerCase())}
                placeholder="home-assistant"
              />
              <small>小写字母、数字和短横线</small>
            </label>
          </div>

          <label>
            <span>说明</span>
            <input value={description} onChange={(event) => setDescription(event.target.value)} placeholder="客厅与全屋设备控制中心" />
          </label>

          <div className="form-grid">
            <label>
              <span>分类</span>
              <select value={category} onChange={(event) => setCategory(event.target.value as ServiceCategory)}>
                {categories.map((item) => <option key={item.id} value={item.id}>{item.label}</option>)}
              </select>
            </label>
            <label>
              <span>图标</span>
              <select value={icon} onChange={(event) => setIcon(event.target.value)}>
                <option value="cpu">设备</option>
                <option value="router">路由器</option>
                <option value="terminal">终端</option>
                <option value="shield">安全</option>
                <option value="sprout">自动化</option>
                <option value="house">智能家居</option>
                <option value="globe">网站</option>
                <option value="pulse">监控</option>
              </select>
            </label>
          </div>

          <label>
            <span>后台实际地址 <em>可选</em></span>
            <input
              type="url"
              value={url}
              onChange={(event) => {
                setURL(event.target.value)
                if (probeType === 'http') setProbeTarget(event.target.value)
              }}
              placeholder="http://192.168.8.178:8123"
            />
          </label>

          <div className="form-grid">
            <label>
              <span>心跳方式</span>
              <select value={probeType} onChange={(event) => {
                const next = event.target.value as ProbeType
                setProbeType(next)
                setProbeTarget(next === 'http' ? url : '')
              }}>
                {(Object.keys(probeLabels) as ProbeType[]).map((type) => <option key={type} value={type}>{probeLabels[type]}</option>)}
              </select>
            </label>
            <label>
              <span>检查目标</span>
              <input required value={probeTarget} onChange={(event) => setProbeTarget(event.target.value)} placeholder={probePlaceholder} />
            </label>
          </div>

          <div className="domain-box">
            <div className="domain-preview">
              <span>默认入口</span>
              <code>{pathPreview}</code>
            </div>
            <label>
              <span>自定义子域名 <em>可选</em></span>
              <div className="domain-input">
                <input
                  pattern="[a-z0-9](?:(?:[a-z0-9]|-)*[a-z0-9])?"
                  maxLength={63}
                  value={subdomain}
                  onChange={(event) => setSubdomain(event.target.value.toLowerCase())}
                  placeholder="ha"
                />
                <span>.{hostname}</span>
              </div>
            </label>
            {subdomainPreview && <div className="domain-preview domain-preview--accent"><span>自定义入口</span><code>{subdomainPreview}</code></div>}
            <label className={`check-row ${!subdomain ? 'check-row--disabled' : ''}`}>
              <input type="checkbox" checked={proxy} disabled={!subdomain} onChange={(event) => setProxy(event.target.checked)} />
              <span><strong>保持自定义域名</strong><small>通过 MeowDeck 反向代理页面；部分后台可能不兼容。</small></span>
            </label>
          </div>

          {error && <div className="form-error" role="alert">{error}</div>}
          <div className="form-actions">
            <button className="secondary-button" type="button" onClick={onClose}>取消</button>
            <button className="primary-button" type="submit" disabled={saving}>{saving ? '正在保存…' : '添加服务'}</button>
          </div>
        </form>
      </motion.section>
    </div>
  )
}

export default function App() {
  const [status, setStatus] = useState<StatusResponse>(demoStatus)
  const [isDemo, setIsDemo] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [panelOpen, setPanelOpen] = useState(false)
  const [deletingID, setDeletingID] = useState('')
  const [actionError, setActionError] = useState('')

  const refresh = useCallback(async () => {
    setRefreshing(true)
    try {
      const result = await fetchStatus()
      setStatus(result.data)
      setIsDemo(result.demo)
    } finally {
      setRefreshing(false)
    }
  }, [])

  useEffect(() => {
    const initial = window.setTimeout(() => void refresh(), 0)
    const timer = window.setInterval(() => void refresh(), POLL_INTERVAL_MS)
    return () => {
      window.clearTimeout(initial)
      window.clearInterval(timer)
    }
  }, [refresh])

  const groups = useMemo(
    () => categories.map((category) => ({
      ...category,
      services: status.services.filter((service) => service.category === category.id),
    })).filter((group) => group.services.length > 0),
    [status.services],
  )

  const activeServices = status.services.filter((service) => service.state !== 'disabled')
  const healthyServices = activeServices.filter((service) => service.state === 'online').length
  const allHealthy = activeServices.length > 0 && healthyServices === activeServices.length

  async function removeService(service: ServiceStatus) {
    if (!window.confirm(`删除“${service.name}”及其入口吗？`)) return
    setDeletingID(service.id)
    setActionError('')
    try {
      await deleteService(service.id)
      await refresh()
    } catch (error) {
      setActionError(error instanceof Error ? error.message : '删除失败')
    } finally {
      setDeletingID('')
    }
  }

  return (
    <main className="app-shell">
      <div className="ambient ambient--one" aria-hidden="true" />
      <div className="ambient ambient--two" aria-hidden="true" />

      <header className="topbar">
        <a className="brand" href="/" aria-label="MeowDeck 首页">
          <span className="brand-mark"><Cat size={22} aria-hidden="true" /></span>
          <span><strong>MeowDeck</strong><small>本地服务控制台</small></span>
        </a>
        <div className="topbar-actions">
          <span className="domain-chip"><RadioTower size={14} aria-hidden="true" /> {status.hostname}</span>
          <button className="add-button" type="button" onClick={() => setPanelOpen(true)}><Plus size={16} /> 添加服务</button>
          <button className="icon-button" type="button" onClick={() => void refresh()} disabled={refreshing} aria-label="刷新状态">
            <RefreshCw size={17} className={refreshing ? 'spin' : ''} aria-hidden="true" />
          </button>
        </div>
      </header>

      <section className="hero" aria-labelledby="hero-title">
        <div>
          <span className={`system-status ${allHealthy ? 'system-status--ok' : 'system-status--warn'}`}>
            {allHealthy ? <Wifi size={15} aria-hidden="true" /> : <WifiOff size={15} aria-hidden="true" />}
            {allHealthy ? '核心服务正常' : '部分服务需要关注'}
          </span>
          <h1 id="hero-title">家里的后台，一眼看清。</h1>
          <p>设备入口、实时心跳与状态历史集中在一个安静的本地页面。</p>
        </div>
        <div className="hero-stats" aria-label="服务概览">
          <div><strong>{activeServices.length}</strong><span>已监测</span></div>
          <div><strong>{healthyServices}</strong><span>在线</span></div>
          <div><strong>{status.intervalSeconds}s</strong><span>巡检</span></div>
        </div>
      </section>

      {isDemo && <div className="demo-banner">当前显示设计预览；连接 MeowDeck API 后自动切换为实时状态。</div>}
      {actionError && <div className="form-error action-error" role="alert">{actionError}</div>}

      <div className="service-sections">
        {groups.map((group) => (
          <section className="service-section" key={group.id} aria-labelledby={`category-${group.id}`}>
            <div className="section-heading">
              <div><h2 id={`category-${group.id}`}>{group.label}</h2><p>{group.hint}</p></div>
              <span>{group.services.length} 项</span>
            </div>
            <div className="service-grid">
              {group.services.map((service, index) => (
                <ServiceCard
                  service={service}
                  hostname={status.hostname}
                  index={index}
                  deleting={deletingID === service.id}
                  onDelete={(item) => void removeService(item)}
                  key={service.id}
                />
              ))}
            </div>
          </section>
        ))}
      </div>

      <button className="empty-add" type="button" onClick={() => setPanelOpen(true)}><Plus size={17} /> 添加下一项服务</button>

      <footer><span>MeowDeck {status.version}</span><span>数据更新于 {relativeTime(status.generatedAt)}</span></footer>
      {panelOpen && <AddServicePanel hostname={status.hostname} onClose={() => setPanelOpen(false)} onAdded={refresh} />}
    </main>
  )
}
