import './styles.css'
import { renderCatalog } from './app'
import type { Catalog } from './types'

const root = document.getElementById('root')
if (!root) throw new Error('missing application root')

const shell = document.createElement('main')
const header = document.createElement('header')
const brand = document.createElement('a')
brand.className = 'brand'
brand.href = '/'
brand.textContent = 'MeowDeck'
const search = document.createElement('input')
search.type = 'search'
search.placeholder = '搜索设备或服务'
search.setAttribute('aria-label', '搜索设备或服务')
header.append(brand, search)
const content = document.createElement('div')
content.className = 'content'
content.textContent = '正在发现局域网服务…'
shell.append(header, content)
root.append(shell)

let current: Catalog | undefined

async function refresh(): Promise<void> {
  try {
    const response = await fetch('/api/v1/catalog', { headers: { Accept: 'application/json' } })
    if (!response.ok) throw new Error('HTTP ' + String(response.status))
    current = await response.json() as Catalog
    renderCatalog(content, current, search.value)
  } catch {
    content.replaceChildren()
    const message = document.createElement('p')
    message.className = 'load-error'
    message.textContent = '暂时无法读取自动发现结果，稍后会自动重试。'
    content.append(message)
  }
}

search.addEventListener('input', () => {
  if (current) renderCatalog(content, current, search.value)
})

void refresh()
window.setInterval(() => void refresh(), 30_000)
