import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import * as Dialog from 'radix-ui/dialog'
import * as Label from 'radix-ui/label'
import * as Select from 'radix-ui/select'
import { ChevronDown, Trash2, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { apiFetch, ApiError } from '@/lib/api'
import { cn } from '@/lib/utils'

interface AccountResponse {
  id: string
  provider: string
  label: string
  meta: Record<string, unknown>
  created_at: string
}

interface ProviderErrorPayload {
  kind: string
  message: string
}

const PROVIDERS = [
  { value: 'github', label: 'GitHub' },
  { value: 'cloudflare', label: 'Cloudflare' },
  { value: 'vercel', label: 'Vercel' },
] as const

function ProviderIcon({ provider }: { provider: string }) {
  const fallback = provider.charAt(0).toUpperCase()
  const colors: Record<string, string> = {
    github: 'bg-pale-green text-green-900',
    cloudflare: 'bg-pale-red text-red-900',
    vercel: 'bg-pale-blue text-blue-900',
  }
  return (
    <div
      className={cn(
        'flex size-7 items-center justify-center rounded-md text-xs font-semibold',
        colors[provider] ?? 'bg-muted text-muted-foreground',
      )}
    >
      {fallback}
    </div>
  )
}

function MetaSummary({ provider, meta }: { provider: string; meta?: Record<string, unknown> }) {
  const m = meta ?? {}
  if (provider === 'github') {
    const login = m.login as string | undefined
    const scopes = m.scopes as string[] | undefined
    const missing = m.missing_scopes as string[] | undefined
    return (
      <div className="flex flex-col gap-0.5">
        {login && <span className="font-medium text-foreground">{login}</span>}
        {scopes && (
          <span className="text-xs text-muted-foreground">{scopes.join(', ')}</span>
        )}
        {missing && missing.length > 0 && (
          <span className="inline-flex w-fit items-center rounded-full bg-pale-yellow px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wider text-yellow-900">
            missing: {missing.join(', ')}
          </span>
        )}
      </div>
    )
  }
  if (provider === 'cloudflare') {
    const name = meta?.account_name as string | undefined
    return (
      <span className="font-medium text-foreground">{name ?? meta?.account_id as string}</span>
    )
  }
  if (provider === 'vercel') {
    const username = meta?.username as string | undefined
    const teamId = meta?.team_id as string | undefined
    return (
      <div className="flex flex-col gap-0.5">
        {username && <span className="font-medium text-foreground">{username}</span>}
        {teamId && <span className="text-xs text-muted-foreground">team: {teamId}</span>}
      </div>
    )
  }
  return <span className="text-muted-foreground">—</span>
}

function useAccounts() {
  return useQuery<AccountResponse[]>({
    queryKey: ['accounts'],
    queryFn: () => apiFetch<AccountResponse[]>('/accounts'),
  })
}

function useAddAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: { provider: string; label: string; token: string }) =>
      apiFetch<AccountResponse>('/accounts', {
        method: 'POST',
        body: JSON.stringify(body),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['accounts'] })
    },
  })
}

function useDeleteAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      apiFetch<void>(`/accounts/${id}`, { method: 'DELETE' }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['accounts'] })
    },
  })
}

function parseProviderError(error: unknown): ProviderErrorPayload | null {
  if (!(error instanceof ApiError)) return null
  try {
    const parsed = JSON.parse(error.message)
    if (typeof parsed.kind === 'string' && typeof parsed.message === 'string') {
      return parsed as ProviderErrorPayload
    }
  } catch {}
  return { kind: 'error', message: error.message }
}

function AddAccountDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const [provider, setProvider] = useState('')
  const [label, setLabel] = useState('')
  const [token, setToken] = useState('')
  const [error, setError] = useState<string | null>(null)

  const addAccount = useAddAccount()

  function reset() {
    setProvider('')
    setLabel('')
    setToken('')
    setError(null)
  }

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setError(null)
    if (!provider || !token) {
      setError('Provider and token are required.')
      return
    }
    try {
      await addAccount.mutateAsync({ provider, label, token })
      reset()
      onOpenChange(false)
    } catch (err) {
      const parsed = parseProviderError(err)
      setError(parsed?.message ?? 'An unexpected error occurred.')
    }
  }

  return (
    <Dialog.Root
      open={open}
      onOpenChange={(open) => {
        if (!open) reset()
        onOpenChange(open)
      }}
    >
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-50 bg-black/30" />
        <Dialog.Content className="fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-xl bg-background p-6 ring-1 ring-foreground/10">
          <div className="mb-5 flex items-center justify-between">
            <Dialog.Title className="font-heading text-lg font-medium tracking-tight">
              Add Account
            </Dialog.Title>
            <Dialog.Close asChild>
              <button className="flex size-6 items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground">
                <X size={14} />
              </button>
            </Dialog.Close>
          </div>

          <form onSubmit={handleSubmit} className="flex flex-col gap-4">
            <div className="flex flex-col gap-1.5">
              <Label.Root className="text-sm font-medium text-foreground">
                Provider
              </Label.Root>
              <Select.Root value={provider} onValueChange={setProvider}>
                <Select.Trigger className="flex h-8 w-full items-center justify-between rounded-lg border border-input bg-transparent px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50">
                  <Select.Value placeholder="Select provider" />
                  <Select.Icon>
                    <ChevronDown size={14} />
                  </Select.Icon>
                </Select.Trigger>
                <Select.Portal>
                  <Select.Content className="z-50 rounded-lg border border-border bg-background p-1 shadow-sm">
                    {PROVIDERS.map((p) => (
                      <Select.Item
                        key={p.value}
                        value={p.value}
                        className="flex h-8 cursor-default items-center rounded-md px-2.5 text-sm outline-none hover:bg-muted data-[highlighted]:bg-muted"
                      >
                        <Select.ItemText>{p.label}</Select.ItemText>
                      </Select.Item>
                    ))}
                  </Select.Content>
                </Select.Portal>
              </Select.Root>
            </div>

            <div className="flex flex-col gap-1.5">
              <Label.Root className="text-sm font-medium text-foreground">
                Label
              </Label.Root>
              <Input
                placeholder="my-account"
                value={label}
                onChange={(e) => setLabel(e.target.value)}
              />
            </div>

            <div className="flex flex-col gap-1.5">
              <Label.Root className="text-sm font-medium text-foreground">
                Token
              </Label.Root>
              <Input
                type="password"
                placeholder="Paste your API token"
                value={token}
                onChange={(e) => setToken(e.target.value)}
                autoComplete="off"
              />
            </div>

            {error && (
              <div className="rounded-lg border border-pale-red bg-pale-red/50 px-3 py-2 text-sm text-red-900">
                {error}
              </div>
            )}

            <div className="flex justify-end gap-2 pt-1">
              <Dialog.Close asChild>
                <Button type="button" variant="outline">
                  Cancel
                </Button>
              </Dialog.Close>
              <Button type="submit" disabled={addAccount.isPending}>
                {addAccount.isPending ? 'Adding…' : 'Add Account'}
              </Button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

function DeleteAccountDialog({
  account,
  open,
  onOpenChange,
}: {
  account: AccountResponse | null
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const deleteAccount = useDeleteAccount()
  const [error, setError] = useState<string | null>(null)

  async function handleConfirm() {
    if (!account) return
    setError(null)
    try {
      await deleteAccount.mutateAsync(account.id)
      onOpenChange(false)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to delete account.')
    }
  }

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-50 bg-black/30" />
        <Dialog.Content className="fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-xl bg-background p-6 ring-1 ring-foreground/10">
          <div className="mb-5 flex items-center justify-between">
            <Dialog.Title className="font-heading text-lg font-medium tracking-tight">
              Delete Account
            </Dialog.Title>
            <Dialog.Close asChild>
              <button className="flex size-6 items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground">
                <X size={14} />
              </button>
            </Dialog.Close>
          </div>

          <div className="flex flex-col gap-3">
            <p className="text-sm text-foreground">
              Are you sure you want to delete{' '}
              <span className="font-medium">{account?.label}</span>?
            </p>
            <p className="rounded-lg border border-border bg-muted/50 px-3 py-2 text-sm text-muted-foreground">
              This will unbind all associated bindings. Remote resources will not be affected.
            </p>

            {error && (
              <div className="rounded-lg border border-pale-red bg-pale-red/50 px-3 py-2 text-sm text-red-900">
                {error}
              </div>
            )}

            <div className="flex justify-end gap-2 pt-1">
              <Dialog.Close asChild>
                <Button type="button" variant="outline">
                  Cancel
                </Button>
              </Dialog.Close>
              <Button
                variant="destructive"
                onClick={handleConfirm}
                disabled={deleteAccount.isPending}
              >
                {deleteAccount.isPending ? 'Deleting…' : 'Delete'}
              </Button>
            </div>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

function Accounts() {
  const [addOpen, setAddOpen] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<AccountResponse | null>(null)

  const { data: accounts, isLoading, error } = useAccounts()

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="font-heading text-2xl font-medium tracking-tight">
            Accounts
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Connect API tokens for GitHub, Cloudflare, and Vercel.
          </p>
        </div>
        <Button onClick={() => setAddOpen(true)}>Add Account</Button>
      </div>

      {isLoading && (
        <div className="flex flex-col gap-2">
          {[1, 2, 3].map((i) => (
            <div key={i} className="flex h-14 animate-pulse items-center gap-3 rounded-lg border border-border bg-muted/30 px-4" />
          ))}
        </div>
      )}

      {error && (
        <div className="rounded-lg border border-pale-red bg-pale-red/50 px-4 py-3 text-sm text-red-900">
          Failed to load accounts. Please try again.
        </div>
      )}

      {accounts && accounts.length === 0 && !isLoading && (
        <div className="mt-8 flex flex-col items-center gap-2 py-12 text-center">
          <p className="text-sm text-muted-foreground">No accounts configured yet.</p>
          <Button variant="outline" onClick={() => setAddOpen(true)}>
            Add your first account
          </Button>
        </div>
      )}

      {accounts && accounts.length > 0 && (
        <div className="overflow-hidden rounded-xl border border-border">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border bg-muted/30 text-left text-xs font-medium uppercase tracking-wider text-muted-foreground">
                <th className="px-4 py-3 pl-4">Provider</th>
                <th className="px-4 py-3">Label</th>
                <th className="px-4 py-3">Identity / Scopes</th>
                <th className="px-4 py-3">Created</th>
                <th className="w-12 px-4 py-3" />
              </tr>
            </thead>
            <tbody>
              {accounts.map((acct) => (
                <tr
                  key={acct.id}
                  className="border-b border-border last:border-b-0 hover:bg-muted/20"
                >
                  <td className="px-4 py-3 pl-4">
                    <div className="flex items-center gap-2">
                      <ProviderIcon provider={acct.provider} />
                      <span className="font-medium capitalize text-foreground">
                        {acct.provider}
                      </span>
                    </div>
                  </td>
                  <td className="px-4 py-3 text-foreground">{acct.label}</td>
                  <td className="px-4 py-3">
                    <MetaSummary provider={acct.provider} meta={acct.meta} />
                  </td>
                  <td className="px-4 py-3 text-muted-foreground">
                    {new Date(acct.created_at).toLocaleDateString()}
                  </td>
                  <td className="px-4 py-3">
                    <button
                      className="flex size-7 items-center justify-center rounded-md text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
                      onClick={() => setDeleteTarget(acct)}
                    >
                      <Trash2 size={14} />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <AddAccountDialog open={addOpen} onOpenChange={setAddOpen} />
      <DeleteAccountDialog
        account={deleteTarget}
        open={deleteTarget !== null}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null)
        }}
      />
    </div>
  )
}

export default Accounts