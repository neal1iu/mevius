import { useCallback, useEffect, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useParams } from 'react-router-dom'
import {
  type AccountResponse,
  type Binding,
  type ExternalResource,
  bindResource,
  discoverResources,
  getSlot,
  listAccounts,
  listBindings,
  refreshBinding,
  unbindBinding,
} from '@/lib/api'
import { DnsRecords } from '@/components/DnsRecords'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'

const CAPABILITY_MATRIX: Record<string, string[]> = {
  repo: ['github'],
  compute: ['cloudflare'],
  'static-site': ['cloudflare', 'vercel'],
  'dns-domain': ['cloudflare', 'vercel'],
}

const SLOT_KIND_BY_ROLE: Record<string, string> = {
  source: 'repo',
  backend: 'compute',
  database: 'compute',
  frontend: 'static-site',
  dns: 'dns-domain',
}

const PRODUCT_BY_PROVIDER_AND_KIND: Record<string, Record<string, string>> = {
  github: { repo: 'github.repositories' },
  cloudflare: {
    compute: 'cloudflare.workers',
    'static-site': 'cloudflare.pages',
    'dns-domain': 'cloudflare.dns',
  },
  vercel: {
    'static-site': 'vercel.projects',
    'dns-domain': 'vercel.dns',
  },
}

const PROVIDER_LABELS: Record<string, string> = {
  github: 'GitHub',
  cloudflare: 'Cloudflare',
  vercel: 'Vercel',
}

function relativeTime(dateStr: string | undefined | null): string {
  if (!dateStr) return 'never'
  const now = Date.now()
  const then = new Date(dateStr).getTime()
  const diff = now - then
  const seconds = Math.floor(diff / 1000)
  if (seconds < 60) return 'just now'
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  if (days < 30) return `${days}d ago`
  return new Date(dateStr).toLocaleDateString()
}

const STATUS_CONFIG: Record<
  string,
  { label: string; bg: string; text: string }
> = {
  ok: { label: 'ok', bg: 'bg-[#EDF3EC]', text: 'text-[#346538]' },
  error: { label: 'error', bg: 'bg-[#FDEBEC]', text: 'text-[#9F2F2D]' },
  auth_error: { label: 'auth error', bg: 'bg-[#FEF3E2]', text: 'text-[#9A5600]' },
  orphaned: { label: 'remote deleted', bg: 'bg-[#F3F3F2]', text: 'text-[#787774]' },
  never: { label: 'never synced', bg: 'bg-[#E1F3FE]', text: 'text-[#1F6C9F]' },
}

function StatusBadge({ status }: { status: string }) {
  const cfg = STATUS_CONFIG[status] ?? STATUS_CONFIG.never
  return (
    <span
      className={cn(
        'inline-flex items-center rounded-full px-2 py-0.5 text-[11px] font-medium uppercase tracking-wider',
        cfg.bg,
        cfg.text,
      )}
    >
      {cfg.label}
    </span>
  )
}

function extractDisplayName(binding: Binding): string {
  if (binding.cached_meta && typeof binding.cached_meta.name === 'string') {
    return binding.cached_meta.name
  }
  return binding.external_id
}

function providerFromAccountId(
  accounts: AccountResponse[],
  accountId: string,
): string {
  return accounts.find((a) => a.id === accountId)?.provider ?? 'unknown'
}

function ProviderLabel({
  provider,
  className,
}: {
  provider: string
  className?: string
}) {
  const label = PROVIDER_LABELS[provider] ?? provider
  const initials = label.slice(0, 2).toUpperCase()
  return (
    <span
      className={cn(
        'inline-flex size-7 items-center justify-center rounded-md bg-muted text-[10px] font-bold tracking-tight',
        className,
      )}
      title={label}
    >
      {initials}
    </span>
  )
}

function Toast({
  message,
  type,
  onClose,
}: {
  message: string
  type: 'success' | 'error'
  onClose: () => void
}) {
  useEffect(() => {
    const t = setTimeout(onClose, 3000)
    return () => clearTimeout(t)
  }, [onClose])

  return (
    <div
      className={cn(
        'fixed bottom-6 right-6 z-50 flex items-center gap-2 rounded-lg border px-4 py-3 text-sm shadow-sm',
        type === 'success'
          ? 'border-[#346538]/20 bg-[#EDF3EC] text-[#346538]'
          : 'border-[#9F2F2D]/20 bg-[#FDEBEC] text-[#9F2F2D]',
      )}
    >
      {type === 'success' ? '\u2713' : '\u2717'} {message}
    </div>
  )
}

function ConfirmDialog({
  open,
  title,
  message,
  confirmLabel,
  onConfirm,
  onCancel,
}: {
  open: boolean
  title: string
  message: string
  confirmLabel?: string
  onConfirm: () => void
  onCancel: () => void
}) {
  if (!open) return null
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/20">
      <div className="w-full max-w-sm rounded-xl bg-white p-6 shadow-sm ring-1 ring-foreground/10">
        <h3 className="text-base font-medium">{title}</h3>
        <p className="mt-2 text-sm text-muted-foreground">{message}</p>
        <div className="mt-6 flex justify-end gap-2">
          <Button variant="outline" size="sm" onClick={onCancel}>
            Cancel
          </Button>
          <Button variant="destructive" size="sm" onClick={onConfirm}>
            {confirmLabel ?? 'Confirm'}
          </Button>
        </div>
      </div>
    </div>
  )
}

function BindDialog({
  open,
  slotKind,
  onClose,
  onBound,
}: {
  open: boolean
  slotKind: string
  onClose: () => void
  onBound: () => void
}) {
  const queryClient = useQueryClient()
  const { data: accounts } = useQuery({
    queryKey: ['accounts'],
    queryFn: listAccounts,
    enabled: open,
  })
  const { data: discoverResult, mutateAsync: doDiscover, isPending: isDiscovering } = useMutation({
    mutationFn: ({ connectionId, product }: { connectionId: string; product: string }) =>
      discoverResources(connectionId, product),
  })

  const { mutateAsync: doBind, isPending: isBinding } = useMutation({
    mutationFn: ({ connectionId, product, externalId }: { connectionId: string; product: string; externalId: string }) => {
      const slotId = window.location.pathname.split('/').pop()!
      return bindResource(slotId, connectionId, product, externalId)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['slot-bindings'] })
      onBound()
      onClose()
    },
  })

  const allowedProviders = CAPABILITY_MATRIX[slotKind] ?? []
  const filteredAccounts =
    accounts?.filter((a) => allowedProviders.includes(a.provider)) ?? []

  const [selectedAccountId, setSelectedAccountId] = useState('')
  const [selectedExternalId, setSelectedExternalId] = useState('')
  const [search, setSearch] = useState('')
  const selectedAccount = accounts?.find((account) => account.id === selectedAccountId)
  const selectedProduct = selectedAccount
    ? PRODUCT_BY_PROVIDER_AND_KIND[selectedAccount.provider]?.[slotKind]
    : undefined

  useEffect(() => {
    setSelectedAccountId('')
    setSelectedExternalId('')
    setSearch('')
  }, [open])

  useEffect(() => {
    if (selectedAccountId && selectedProduct) {
      doDiscover({ connectionId: selectedAccountId, product: selectedProduct })
    }
  }, [selectedAccountId, selectedProduct, doDiscover])

  const handleSubmit = useCallback(async () => {
    if (!selectedAccountId || !selectedProduct || !selectedExternalId) return
    await doBind({ connectionId: selectedAccountId, product: selectedProduct, externalId: selectedExternalId })
  }, [selectedAccountId, selectedProduct, selectedExternalId, doBind])

  const resources: ExternalResource[] = discoverResult ?? []
  const searchedResources = search
    ? resources.filter(
        (r) =>
          r.display_name.toLowerCase().includes(search.toLowerCase()) ||
          r.external_id.toLowerCase().includes(search.toLowerCase()),
      )
    : resources

  if (!open) return null

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/20">
      <div className="w-full max-w-lg rounded-xl bg-white p-6 shadow-sm ring-1 ring-foreground/10">
        <h3 className="text-base font-medium">Bind Resource</h3>

        <label className="mt-4 block text-sm font-medium text-muted-foreground">
          Account
        </label>
        <div className="mt-1 grid grid-cols-1 gap-1">
          {filteredAccounts.map((acct) => (
            <button
              key={acct.id}
              type="button"
              onClick={() => setSelectedAccountId(acct.id)}
              className={cn(
                'flex items-center gap-2 rounded-lg border px-3 py-2 text-left text-sm transition-colors',
                selectedAccountId === acct.id
                  ? 'border-foreground bg-muted'
                  : 'border-border hover:bg-muted',
              )}
            >
              <ProviderLabel provider={acct.provider} />
              <span>{acct.label}</span>
              <span className="ml-auto text-xs text-muted-foreground">
                {PROVIDER_LABELS[acct.provider] ?? acct.provider}
              </span>
            </button>
          ))}
          {filteredAccounts.length === 0 && (
            <p className="py-2 text-sm text-muted-foreground">
              No compatible accounts found.
            </p>
          )}
        </div>

        {selectedAccountId && (
          <>
            <label className="mt-4 block text-sm font-medium text-muted-foreground">
              Search remote resources
            </label>
            <Input
              placeholder="Type to filter..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="mt-1"
            />

            <div className="mt-2 max-h-60 space-y-1 overflow-y-auto">
              {isDiscovering && (
                <p className="py-4 text-center text-sm text-muted-foreground">
                  Loading...
                </p>
              )}
              {!isDiscovering &&
                searchedResources.map((r) => (
                  <button
                    key={r.external_id}
                    type="button"
                    onClick={() => setSelectedExternalId(r.external_id)}
                    className={cn(
                      'flex w-full items-center gap-2 rounded-lg border px-3 py-2 text-left text-sm transition-colors',
                      selectedExternalId === r.external_id
                        ? 'border-foreground bg-muted'
                        : 'border-border hover:bg-muted',
                    )}
                  >
                    <span className="flex size-4 shrink-0 items-center justify-center rounded-full border border-muted-foreground">
                      {selectedExternalId === r.external_id && (
                        <span className="size-2 rounded-full bg-foreground" />
                      )}
                    </span>
                    <span className="font-medium">{r.display_name}</span>
                    <span className="ml-auto text-xs text-muted-foreground">
                      {r.external_id}
                    </span>
                  </button>
                ))}
              {!isDiscovering && searchedResources.length === 0 && (
                <p className="py-4 text-center text-sm text-muted-foreground">
                  {discoverResult
                    ? 'No results match your search.'
                    : 'No resources found.'}
                </p>
              )}
            </div>
          </>
        )}

        <div className="mt-6 flex justify-end gap-2">
          <Button variant="outline" size="sm" onClick={onClose}>
            Cancel
          </Button>
          <Button
            size="sm"
            disabled={!selectedAccountId || !selectedExternalId || isBinding}
            onClick={handleSubmit}
          >
            {isBinding ? 'Binding...' : 'Bind'}
          </Button>
        </div>
      </div>
    </div>
  )
}

function BindingCard({
  binding,
  accounts,
  onRefresh,
  onUnbind,
  isRefreshing,
}: {
  binding: Binding
  accounts: AccountResponse[]
  onRefresh: () => void
  onUnbind: () => void
  isRefreshing: boolean
}) {
  const provider = providerFromAccountId(accounts, binding.connection_id)
  const displayName = extractDisplayName(binding)
  const isOrphaned = binding.sync_status === 'orphaned'

  return (
    <Card size="sm">
      <CardHeader>
        <div className="flex items-center gap-2">
          <ProviderLabel provider={provider} />
          <CardTitle className="truncate">{displayName}</CardTitle>
          <div className="ml-auto">
            <StatusBadge status={binding.sync_status} />
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-1">
        <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <span className="font-medium text-foreground">
            {PROVIDER_LABELS[provider] ?? provider}
          </span>
          <span>/</span>
          <span className="truncate font-mono">{binding.external_id}</span>
        </div>
        <div className="flex items-center gap-1 text-xs text-muted-foreground">
          synced {relativeTime(binding.last_synced_at)}
        </div>
      </CardContent>
      <CardFooter className="gap-2">
        {isOrphaned && (
          <span className="mr-auto text-[11px] text-muted-foreground">
            Remote resource was deleted. Unbind to remove this binding.
          </span>
        )}
        <div className="ml-auto flex gap-1.5">
          <Button
            variant="outline"
            size="xs"
            disabled={isRefreshing}
            onClick={onRefresh}
          >
            {isRefreshing ? 'Refreshing...' : 'Refresh'}
          </Button>
          <Button
            variant="destructive"
            size="xs"
            onClick={onUnbind}
          >
            Unbind
          </Button>
        </div>
      </CardFooter>
    </Card>
  )
}

function SlotDetail() {
  const { slotId } = useParams<{ projectId: string; slotId: string }>()

  const { data: slot, isLoading: slotLoading } = useQuery({
    queryKey: ['slot', slotId],
    queryFn: () => getSlot(slotId!),
    enabled: !!slotId,
  })

  const { data: accounts } = useQuery({
    queryKey: ['accounts'],
    queryFn: listAccounts,
  })

  const {
    data: bindings,
    isLoading: bindingsLoading,
    refetch: refetchBindings,
  } = useQuery({
    queryKey: ['slot-bindings', slotId],
    queryFn: () => listBindings(slotId!),
    enabled: !!slotId,
  })

  const { mutateAsync: doRefresh } = useMutation({
    mutationFn: (bindingId: string) => refreshBinding(bindingId),
  })

  const { mutateAsync: doUnbind } = useMutation({
    mutationFn: (bindingId: string) => unbindBinding(bindingId),
  })

  const [refreshingIds, setRefreshingIds] = useState<Set<string>>(new Set())
  const [unbindTarget, setUnbindTarget] = useState<Binding | null>(null)
  const [bindDialogOpen, setBindDialogOpen] = useState(false)
  const [toast, setToast] = useState<{
    message: string
    type: 'success' | 'error'
    id: number
  } | null>(null)
  const toastId = useRef(0)

  const showToast = useCallback((message: string, type: 'success' | 'error') => {
    toastId.current++
    setToast({ message, type, id: toastId.current })
  }, [])

  const handleRefresh = useCallback(
    async (binding: Binding) => {
      setRefreshingIds((prev) => new Set(prev).add(binding.id))
      try {
        await doRefresh(binding.id)
        showToast('Binding refreshed successfully', 'success')
        refetchBindings()
      } catch {
        showToast('Failed to refresh binding', 'error')
      } finally {
        setRefreshingIds((prev) => {
          const next = new Set(prev)
          next.delete(binding.id)
          return next
        })
      }
    },
    [doRefresh, refetchBindings, showToast],
  )

  const handleUnbind = useCallback(async () => {
    if (!unbindTarget) return
    try {
      await doUnbind(unbindTarget.id)
      showToast('Binding removed', 'success')
      refetchBindings()
    } catch {
      showToast('Failed to unbind', 'error')
    } finally {
      setUnbindTarget(null)
    }
  }, [unbindTarget, doUnbind, refetchBindings, showToast])

  if (slotLoading) {
    return (
      <div>
        <h1 className="font-heading text-2xl font-medium tracking-tight">
          Slot
        </h1>
        <p className="mt-2 text-sm text-muted-foreground">Loading...</p>
      </div>
    )
  }

  const bindingList: Binding[] = bindings ?? []
  const slotKind = slot ? (SLOT_KIND_BY_ROLE[slot.role] ?? slot.role) : ''

  return (
    <div>
      <div className="flex items-center justify-between">
        <div>
          <h1 className="font-heading text-2xl font-medium tracking-tight">
            {slot?.name ?? 'Slot'}
          </h1>
          <p className="mt-0.5 text-sm text-muted-foreground">
            {slotKind}
          </p>
        </div>
        <Button size="sm" onClick={() => setBindDialogOpen(true)}>
          Bind Resource
        </Button>
      </div>

      <div className="mt-8">
        {bindingsLoading && (
          <p className="text-sm text-muted-foreground">Loading bindings...</p>
        )}
        {!bindingsLoading && bindingList.length === 0 && (
          <div className="rounded-xl border border-dashed border-border p-10 text-center">
            <p className="text-sm text-muted-foreground">
              No bindings yet. Click "Bind Resource" to connect an external
              resource.
            </p>
          </div>
        )}
        {bindingList.length > 0 && (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            {bindingList.map((b) => (
              <BindingCard
                key={b.id}
                binding={b}
                accounts={accounts ?? []}
                onRefresh={() => handleRefresh(b)}
                onUnbind={() => setUnbindTarget(b)}
                isRefreshing={refreshingIds.has(b.id)}
              />
            ))}
          </div>
        )}
      </div>

      {slot && slotKind === 'dns-domain' && bindingList.length > 0 && (
        <div className="mt-8">
          <Card>
            <CardHeader>
              <CardTitle>DNS Records</CardTitle>
            </CardHeader>
            <CardContent>
              <DnsRecords
                bindingId={bindingList[0].id}
                provider={providerFromAccountId(accounts ?? [], bindingList[0].connection_id)}
                verified={
                  bindingList[0].cached_meta
                    ? (bindingList[0].cached_meta['verified'] as boolean | undefined)
                    : undefined
                }
              />
            </CardContent>
          </Card>
        </div>
      )}

      {slot && (
        <BindDialog
          open={bindDialogOpen}
          slotKind={slotKind}
          onClose={() => setBindDialogOpen(false)}
          onBound={() => showToast('Resource bound successfully', 'success')}
        />
      )}

      <ConfirmDialog
        open={unbindTarget !== null}
        title="Unbind resource"
        message="This only unbinds the binding — it does not delete the remote resource."
        confirmLabel="Unbind"
        onConfirm={handleUnbind}
        onCancel={() => setUnbindTarget(null)}
      />

      {toast && (
        <Toast
          key={toast.id}
          message={toast.message}
          type={toast.type}
          onClose={() => setToast(null)}
        />
      )}
    </div>
  )
}

export default SlotDetail
