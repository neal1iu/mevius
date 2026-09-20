import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import Accounts from './Accounts'

function json(value: unknown) {
  return new Response(JSON.stringify(value), { status: 200, headers: { 'content-type': 'application/json' } })
}

const configuredGitHub = { provider_id: 'github', available: true, configured: true, source: 'database', client_id: 'client-id', authorization_url: 'https://github.com/login/oauth/authorize', token_url: 'https://github.com/login/oauth/access_token', scopes: ['repo', 'workflow'], pkce: true, redirect_base_url: 'https://mevius.example', callback_url: 'https://mevius.example/api/v1/connections/oauth/callback/github' }

function renderAccounts() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(<QueryClientProvider client={client}><Accounts /></QueryClientProvider>)
}

beforeEach(() => {
  window.history.replaceState({}, '', '/accounts')
  vi.restoreAllMocks()
  vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
    const path = String(input)
    if (path.endsWith('/connections')) return json([])
    if (path.endsWith('/catalog/providers')) return json({ providers: [{ id: 'github', display_name: 'GitHub', products: [] }] })
    if (path.endsWith('/connections/oauth/providers')) return json({ providers: [configuredGitHub] })
    throw new Error(`unexpected request: ${path}`)
  })
})

describe('Accounts OAuth flow', () => {
  it('offers a configured provider OAuth redirect flow', async () => {
    const user = userEvent.setup()
    renderAccounts()

    await user.click(screen.getByRole('button', { name: 'New connection' }))
    const oauth = await screen.findByRole('button', { name: 'OAuth' })
    expect(oauth).toBeEnabled()
    await user.click(oauth)
    expect(screen.getByRole('button', { name: /Continue to authorization/ })).toBeEnabled()
    expect(screen.queryByLabelText('Token')).not.toBeInTheDocument()
  })

  it('restores an authorized server-side session without rendering credentials', async () => {
    window.history.replaceState({}, '', '/accounts?oauth_session=session-1')
    vi.mocked(fetch).mockImplementation(async (input) => {
      const path = String(input)
      if (path.endsWith('/connections')) return json([])
      if (path.endsWith('/catalog/providers')) return json({ providers: [{ id: 'github', display_name: 'GitHub', products: [] }] })
      if (path.endsWith('/connections/oauth/providers')) return json({ providers: [configuredGitHub] })
      if (path.endsWith('/connections/oauth/sessions/session-1')) return json({ id: 'session-1', provider_id: 'github', status: 'authorized', scopes: [{ type: 'organization', id: 'acme', label: 'Acme' }], expires_at: '2099-01-01T00:00:00Z' })
      throw new Error(`unexpected request: ${path}`)
    })

    renderAccounts()

    expect(await screen.findByRole('option', { name: 'Acme (organization)' })).toBeInTheDocument()
    expect(screen.queryByLabelText('Token')).not.toBeInTheDocument()
    expect(document.body.textContent).not.toContain('access_token')
  })

  it('opens a prefilled setup form instead of disabling unconfigured OAuth', async () => {
    vi.mocked(fetch).mockImplementation(async (input) => {
      const path = String(input)
      if (path.endsWith('/connections')) return json([])
      if (path.endsWith('/catalog/providers')) return json({ providers: [{ id: 'github', display_name: 'GitHub', products: [] }] })
      if (path.endsWith('/connections/oauth/providers')) return json({ providers: [{ ...configuredGitHub, available: false, configured: false, source: undefined, client_id: undefined, redirect_base_url: undefined, callback_url: undefined }] })
      throw new Error(`unexpected request: ${path}`)
    })
    const user = userEvent.setup()
    renderAccounts()

    await user.click(screen.getByRole('button', { name: 'New connection' }))
    const oauth = await screen.findByRole('button', { name: 'OAuth' })
    expect(oauth).toBeEnabled()
    await user.click(oauth)
    expect(screen.getByLabelText('Client ID')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Advanced settings' }))
    expect(screen.getByLabelText('Authorization URL')).toHaveValue('https://github.com/login/oauth/authorize')
  })
})
