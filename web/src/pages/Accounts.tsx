import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ExternalLink, KeyRound, Plus, Trash2, X } from 'lucide-react'
import { Dialog } from 'radix-ui'
import {
  apiFetch,
  type Catalog,
  type Connection,
  type OAuthAuthorizationSession,
  type OAuthProviderInfo,
  type ProbeResult,
  type ProviderScope,
} from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

type AuthChoice = 'token' | 'oauth'
type OAuthConfigurationForm = { clientID: string; clientSecret: string; redirectBaseURL: string; integrationSlug: string; authorizationURL: string; tokenURL: string; scopes: string; pkce: boolean }

function Accounts() {
  const queryClient = useQueryClient()
  const callbackParams = useMemo(() => new URLSearchParams(window.location.search), [])
  const oauthSessionID = callbackParams.get('oauth_session') ?? ''
  const callbackError = callbackParams.get('oauth_error') ?? ''
  const [open, setOpen] = useState(Boolean(oauthSessionID || callbackError))
  const [authChoice, setAuthChoice] = useState<AuthChoice>(oauthSessionID ? 'oauth' : 'token')
  const [providerID, setProviderID] = useState('github')
  const [label, setLabel] = useState('')
  const [credential, setCredential] = useState('')
  const [endpoint, setEndpoint] = useState('')
  const [probe, setProbe] = useState<ProbeResult | null>(null)
  const [scope, setScope] = useState<ProviderScope | null>(null)
  const [error, setError] = useState(callbackError)
  const [oauthSetup, setOAuthSetup] = useState(false)
  const [oauthAdvanced, setOAuthAdvanced] = useState(false)
  const [oauthConfig, setOAuthConfig] = useState<OAuthConfigurationForm | null>(null)

  const { data: connections = [] } = useQuery({
    queryKey: ['connections'],
    queryFn: () => apiFetch<Connection[]>('/connections'),
  })
  const { data: catalog } = useQuery({
    queryKey: ['catalog'],
    queryFn: () => apiFetch<Catalog>('/catalog/providers'),
  })
  const { data: oauthProviders } = useQuery({
    queryKey: ['oauth-providers'],
    queryFn: () => apiFetch<{ providers: OAuthProviderInfo[] }>('/connections/oauth/providers'),
  })
  const { data: oauthSession, error: oauthSessionError } = useQuery({
    queryKey: ['oauth-session', oauthSessionID],
    queryFn: () => apiFetch<OAuthAuthorizationSession>(`/connections/oauth/sessions/${oauthSessionID}`),
    enabled: Boolean(oauthSessionID),
  })

  const oauthProvider = oauthProviders?.providers.find((item) => item.provider_id === providerID)

  const probeMutation = useMutation({
    mutationFn: () => apiFetch<ProbeResult>('/connections/probe', {
      method: 'POST',
      body: JSON.stringify({ provider_id: providerID, endpoint, credential }),
    }),
    onSuccess: (value) => {
      setProbe(value)
      setScope(value.scopes[0] ?? null)
      setError('')
    },
    onError: (value: Error) => setError(value.message),
  })
  const createTokenMutation = useMutation({
    mutationFn: () => apiFetch<Connection>('/connections', {
      method: 'POST',
      body: JSON.stringify({ provider_id: providerID, label: label || scope?.label, endpoint, scope, credential }),
    }),
    onSuccess: finish,
    onError: (value: Error) => setError(value.message),
  })
  const startOAuthMutation = useMutation({
    mutationFn: () => apiFetch<{ authorization_url: string }>('/connections/oauth/start', {
      method: 'POST',
      body: JSON.stringify({ provider_id: providerID, endpoint }),
    }),
    onSuccess: (value) => window.location.assign(value.authorization_url),
    onError: (value: Error) => setError(value.message),
  })
  const saveOAuthConfigurationMutation = useMutation({
    mutationFn: (value: OAuthConfigurationForm) => apiFetch<OAuthProviderInfo>(`/connections/oauth/providers/${providerID}/configuration`, {
      method: 'PUT',
      body: JSON.stringify({
        client_id: value.clientID,
        client_secret: value.clientSecret || undefined,
        redirect_base_url: value.redirectBaseURL,
        integration_slug: value.integrationSlug || undefined,
        authorization_url: value.authorizationURL,
        token_url: value.tokenURL,
        scopes: value.scopes.split(/[\s,]+/).filter(Boolean),
        pkce: value.pkce,
      }),
    }),
    onSuccess: (value) => {
      queryClient.invalidateQueries({ queryKey: ['oauth-providers'] })
      setOAuthConfig(configurationFrom(value))
      setOAuthSetup(false)
      setError('')
    },
    onError: (value: Error) => setError(value.message),
  })
  const completeOAuthMutation = useMutation({
    mutationFn: () => apiFetch<Connection>('/connections/oauth/complete', {
      method: 'POST',
      body: JSON.stringify({ session_id: oauthSessionID, label: label || scope?.label || oauthSession?.scopes?.[0]?.label, scope: scope ?? oauthSession?.scopes?.[0] }),
    }),
    onSuccess: finish,
    onError: (value: Error) => setError(value.message),
  })
  const deleteMutation = useMutation({
    mutationFn: (id: string) => apiFetch<void>(`/connections/${id}`, { method: 'DELETE' }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['connections'] }),
  })

  function finish() {
    queryClient.invalidateQueries({ queryKey: ['connections'] })
    window.history.replaceState({}, '', '/accounts')
    reset()
    setOpen(false)
  }
  function reset() {
    setCredential('')
    setLabel('')
    setEndpoint('')
    setProbe(null)
    setScope(null)
    setError('')
    setAuthChoice('token')
    setOAuthSetup(false)
    setOAuthAdvanced(false)
    setOAuthConfig(null)
  }

  function chooseOAuth() {
    setAuthChoice('oauth')
    setOAuthConfig(configurationFrom(oauthProvider))
    setOAuthSetup(!oauthProvider?.configured)
    setError('')
  }

  return <div>
    <div className="mb-8 flex items-end justify-between">
      <div>
        <p className="text-xs font-medium uppercase tracking-[.18em] text-muted-foreground">Provider access</p>
        <h1 className="mt-2 text-2xl font-medium">Connections</h1>
        <p className="mt-1 text-sm text-muted-foreground">Authorize with OAuth or a token, then pin the connection to one remote scope.</p>
      </div>
      <Button onClick={() => setOpen(true)}><Plus className="size-4" />New connection</Button>
    </div>
    <div className="grid gap-3">
      {connections.map((connection) => <div key={connection.id} className="flex items-center gap-4 rounded-xl border p-4">
        <div className="flex size-9 items-center justify-center rounded-lg bg-muted text-xs font-semibold uppercase">{connection.provider_id.slice(0, 2)}</div>
        <div className="flex-1">
          <div className="flex items-center gap-2"><p className="font-medium">{connection.label}</p><span className="rounded-full border px-2 py-0.5 text-[10px] uppercase text-muted-foreground">{connection.auth_method}</span></div>
          <p className="text-xs text-muted-foreground">{connection.provider_id} · {connection.scope.type}: {connection.scope.label}</p>
        </div>
        <Button variant="ghost" size="icon" onClick={() => deleteMutation.mutate(connection.id)}><Trash2 className="size-4" /></Button>
      </div>)}
    </div>

    <Dialog.Root open={open} onOpenChange={(value) => { setOpen(value); if (!value) { window.history.replaceState({}, '', '/accounts'); reset() } }}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-black/25" />
        <Dialog.Content className="fixed left-1/2 top-1/2 z-50 max-h-[90vh] w-[min(92vw,500px)] -translate-x-1/2 -translate-y-1/2 overflow-y-auto rounded-xl border bg-background p-6 shadow-xl">
          <div className="mb-5 flex items-center justify-between"><Dialog.Title className="text-lg font-medium">Connect provider</Dialog.Title><Dialog.Close><X className="size-4" /></Dialog.Close></div>
          <div className="space-y-4">
            <label className="block text-sm">Provider
              <select className="mt-1 h-9 w-full rounded-md border bg-background px-3" value={oauthSession?.provider_id ?? providerID} disabled={Boolean(oauthSessionID)} onChange={(event) => { setProviderID(event.target.value); setProbe(null); setScope(null); setAuthChoice('token'); setOAuthSetup(false); setOAuthConfig(null) }}>
                {catalog?.providers.map((item) => <option key={item.id} value={item.id}>{item.display_name}</option>)}
              </select>
            </label>

            {!oauthSessionID && <div className="grid grid-cols-2 gap-2 rounded-lg bg-muted p-1">
              <button className={`rounded-md px-3 py-2 text-sm ${authChoice === 'token' ? 'bg-background shadow-sm' : 'text-muted-foreground'}`} onClick={() => setAuthChoice('token')}><KeyRound className="mr-2 inline size-4" />API token</button>
              <button title={oauthProvider?.configured ? 'Use OAuth' : 'Configure OAuth application'} className={`rounded-md px-3 py-2 text-sm ${authChoice === 'oauth' ? 'bg-background shadow-sm' : 'text-muted-foreground'}`} onClick={chooseOAuth}><ExternalLink className="mr-2 inline size-4" />OAuth</button>
            </div>}

            {!oauthSessionID && <label className="block text-sm">Custom API endpoint (optional)<Input className="mt-1" value={endpoint} onChange={(event) => setEndpoint(event.target.value)} /></label>}

            {authChoice === 'oauth' && !oauthSessionID && oauthSetup && oauthConfig && <OAuthConfiguration
              providerID={providerID}
              value={oauthConfig}
              setValue={setOAuthConfig}
              advanced={oauthAdvanced}
              setAdvanced={setOAuthAdvanced}
              existingSource={oauthProvider?.source}
              callbackURL={`${oauthConfig.redirectBaseURL.replace(/\/$/, '')}/api/v1/connections/oauth/callback/${providerID}`}
              pending={saveOAuthConfigurationMutation.isPending}
              onSubmit={() => saveOAuthConfigurationMutation.mutate(oauthConfig)}
              onCancel={oauthProvider?.configured ? () => setOAuthSetup(false) : undefined}
            />}

            {authChoice === 'oauth' && !oauthSessionID && !oauthSetup && <div className="rounded-lg border p-4">
              <p className="text-sm font-medium">Authorize in {catalog?.providers.find((item) => item.id === providerID)?.display_name}</p>
              <p className="mt-1 text-xs text-muted-foreground">You will return here to choose the exact account, organization, or team. OAuth tokens never pass through browser JavaScript.</p>
              {oauthProvider?.callback_url && <p className="mt-3 break-all rounded-md bg-muted p-2 font-mono text-[11px] text-muted-foreground">Callback: {oauthProvider.callback_url}</p>}
              <div className="mt-4 grid grid-cols-[1fr_auto] gap-2">
                <Button disabled={!oauthProvider?.available || startOAuthMutation.isPending} onClick={() => startOAuthMutation.mutate()}>{startOAuthMutation.isPending ? 'Redirecting…' : 'Continue to authorization'}<ExternalLink className="size-4" /></Button>
                <Button variant="outline" onClick={() => { setOAuthConfig(configurationFrom(oauthProvider)); setOAuthSetup(true) }}>Configure</Button>
              </div>
            </div>}

            {authChoice === 'token' && !oauthSessionID && <>
              <label className="block text-sm">Label<Input className="mt-1" value={label} onChange={(event) => setLabel(event.target.value)} placeholder="Production account" /></label>
              <label className="block text-sm">Token<Input className="mt-1" type="password" value={credential} onChange={(event) => setCredential(event.target.value)} autoComplete="off" /></label>
              {!probe ? <Button className="w-full" disabled={!credential || probeMutation.isPending} onClick={() => probeMutation.mutate()}>{probeMutation.isPending ? 'Probing…' : 'Probe scopes'}</Button> : <ScopeConfirmation scopes={probe.scopes} scope={scope} setScope={setScope} label={label} setLabel={setLabel} onSubmit={() => createTokenMutation.mutate()} pending={createTokenMutation.isPending} />}
            </>}

            {oauthSessionID && oauthSession?.status === 'authorized' && <ScopeConfirmation scopes={oauthSession.scopes ?? []} scope={scope ?? oauthSession.scopes?.[0] ?? null} setScope={setScope} label={label || oauthSession.scopes?.[0]?.label || ''} setLabel={setLabel} onSubmit={() => completeOAuthMutation.mutate()} pending={completeOAuthMutation.isPending} />}
            {oauthSessionID && !oauthSession && !oauthSessionError && !error && <p className="text-sm text-muted-foreground">Loading authorization…</p>}
            {oauthSessionID && oauthSession && oauthSession.status !== 'authorized' && <p className="rounded-lg bg-destructive/10 p-3 text-sm text-destructive">Authorization is {oauthSession.status}. {oauthSession.error}</p>}
            {oauthSessionError && <p className="rounded-lg bg-destructive/10 p-3 text-sm text-destructive">{oauthSessionError.message}</p>}
            {error && <p className="rounded-lg bg-destructive/10 p-3 text-sm text-destructive">{error}</p>}
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  </div>
}

function configurationFrom(provider?: OAuthProviderInfo): OAuthConfigurationForm {
  return {
    clientID: provider?.client_id ?? '',
    clientSecret: '',
    redirectBaseURL: provider?.redirect_base_url ?? window.location.origin,
    integrationSlug: provider?.integration_slug ?? '',
    authorizationURL: provider?.authorization_url ?? '',
    tokenURL: provider?.token_url ?? '',
    scopes: provider?.scopes?.join(' ') ?? '',
    pkce: provider?.pkce ?? true,
  }
}

function OAuthConfiguration({ providerID, value, setValue, advanced, setAdvanced, existingSource, callbackURL, pending, onSubmit, onCancel }: { providerID: string; value: OAuthConfigurationForm; setValue: (value: OAuthConfigurationForm) => void; advanced: boolean; setAdvanced: (value: boolean) => void; existingSource?: string; callbackURL: string; pending: boolean; onSubmit: () => void; onCancel?: () => void }) {
  const update = <K extends keyof OAuthConfigurationForm>(key: K, next: OAuthConfigurationForm[K]) => setValue({ ...value, [key]: next })
  const secretRequired = existingSource !== 'database'
  return <div className="space-y-4 rounded-lg border p-4">
    <div>
      <p className="text-sm font-medium">Configure {providerID} OAuth application</p>
      <p className="mt-1 text-xs text-muted-foreground">Create an OAuth application at the provider, register the callback below, then enter its credentials. The secret is encrypted by the backend and is never returned by the API.</p>
    </div>
    <div className="break-all rounded-md bg-muted p-3">
      <p className="mb-1 text-[10px] font-medium uppercase tracking-wide text-muted-foreground">Callback URL</p>
      <code className="text-xs">{callbackURL}</code>
    </div>
    <label className="block text-sm">Client ID<Input className="mt-1" value={value.clientID} onChange={(event) => update('clientID', event.target.value)} /></label>
    <label className="block text-sm">Client Secret<Input className="mt-1" type="password" value={value.clientSecret} onChange={(event) => update('clientSecret', event.target.value)} placeholder={secretRequired ? 'Required' : 'Leave empty to keep the current secret'} autoComplete="new-password" /></label>
    <label className="block text-sm">Public Mevius URL<Input className="mt-1" value={value.redirectBaseURL} onChange={(event) => update('redirectBaseURL', event.target.value)} placeholder="https://mevius.example.com" /></label>
    {providerID === 'vercel' && <label className="block text-sm">Vercel integration slug<Input className="mt-1" value={value.integrationSlug} onChange={(event) => update('integrationSlug', event.target.value)} /></label>}
    <button className="text-xs font-medium text-muted-foreground underline underline-offset-4" onClick={() => setAdvanced(!advanced)}>{advanced ? 'Hide advanced settings' : 'Advanced settings'}</button>
    {advanced && <div className="space-y-3 rounded-lg bg-muted/50 p-3">
      <label className="block text-sm">Authorization URL<Input className="mt-1" value={value.authorizationURL} onChange={(event) => update('authorizationURL', event.target.value)} /></label>
      <label className="block text-sm">Token URL<Input className="mt-1" value={value.tokenURL} onChange={(event) => update('tokenURL', event.target.value)} /></label>
      <label className="block text-sm">Scopes<Input className="mt-1" value={value.scopes} onChange={(event) => update('scopes', event.target.value)} placeholder="space or comma separated" /></label>
      <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={value.pkce} onChange={(event) => update('pkce', event.target.checked)} />Use PKCE</label>
    </div>}
    <div className="flex gap-2">
      <Button className="flex-1" disabled={!value.clientID || (secretRequired && !value.clientSecret) || !value.redirectBaseURL || pending} onClick={onSubmit}>{pending ? 'Saving…' : 'Save OAuth configuration'}</Button>
      {onCancel && <Button variant="outline" onClick={onCancel}>Cancel</Button>}
    </div>
  </div>
}

function ScopeConfirmation({ scopes, scope, setScope, label, setLabel, onSubmit, pending }: { scopes: ProviderScope[]; scope: ProviderScope | null; setScope: (value: ProviderScope | null) => void; label: string; setLabel: (value: string) => void; onSubmit: () => void; pending: boolean }) {
  return <div className="space-y-4 rounded-lg border p-4">
    <label className="block text-sm">Remote scope<select className="mt-1 h-9 w-full rounded-md border bg-background px-3" value={scope?.id ?? ''} onChange={(event) => setScope(scopes.find((item) => item.id === event.target.value) ?? null)}>{scopes.map((item) => <option key={`${item.type}:${item.id}`} value={item.id}>{item.label} ({item.type})</option>)}</select></label>
    <label className="block text-sm">Connection label<Input className="mt-1" value={label} onChange={(event) => setLabel(event.target.value)} /></label>
    <Button className="w-full" disabled={!scope || pending} onClick={onSubmit}>{pending ? 'Saving…' : 'Save scoped connection'}</Button>
  </div>
}

export default Accounts
