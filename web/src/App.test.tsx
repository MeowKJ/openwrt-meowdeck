import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import App from './App'

describe('App', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')))
  })

  it('renders the local dashboard and router entries', async () => {
    render(<App />)

    expect(screen.getByText('MeowDeck')).toBeInTheDocument()
    expect(screen.getByText('GL.iNet 管理后台')).toBeInTheDocument()
    expect(screen.getByText('LuCI 高级管理')).toBeInTheDocument()
    expect(screen.queryByText('QQ 经典农场')).not.toBeInTheDocument()
    expect(screen.getByText('meow.lan/router')).toBeInTheDocument()
  })

  it('opens an add-service panel with path and subdomain options', () => {
    render(<App />)
    fireEvent.click(screen.getByRole('button', { name: '添加服务' }))

    expect(screen.getByRole('dialog', { name: '添加本地服务' })).toBeInTheDocument()
    expect(screen.getByText('http://meow.lan/项目名')).toBeInTheDocument()
    expect(screen.getByLabelText(/自定义子域名/)).toBeInTheDocument()
  })
})
