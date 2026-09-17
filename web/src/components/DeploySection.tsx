import { useState, useCallback } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import type { Binding } from '@/lib/api'
import { listDeploys, triggerDeploy, type DeployResponse } from '@/lib/api'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { LogDrawer } from '@/components/LogDrawer'

const DEPLOY_STATUS_LABELS: Record<string, string> = {
  queued: 'Queued',
  in_progress: 'In Progress',
  completed: 'Completed',
  failed: 'Failed',
  cancelled: 'Cancelled',
}

function DeployStatusIcon({ status, inProgress }: { status: string; inProgress: boolean }) {
  if (inProgress) {
    return <span className="inline-block size-3 animate-spin rounded-full border-2 border-foreground border-t-transparent" />
  }
  if (status === 'completed') {
    return <span className="inline-flex size-3 items-center justify-center rounded-full bg-[#EDF3EC] text-[8px] text-[#346538] font-bold">&#10003;</span>
  }
  if (status === 'failed' || status === 'cancelled') {
    return <span className="inline-flex size-3 items-center justify-center rounded-full bg-[#FDEBEC] text-[8px] text-[#9F2F2D] font-bold">&#10007;</span>
  }
  return <span className="inline-block size-3 rounded-full border border-muted-foreground" />
}

function relativeTime(dateStr: string | undefined | null): string {
  if (!dateStr) return ''
  const now = Date.now()
  const then = new Date(dateStr).getTime()
  if (isNaN(then)) return dateStr
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
          <Button variant="default" size="sm" onClick={onConfirm}>
            {confirmLabel ?? 'Confirm'}
          </Button>
        </div>
      </div>
    </div>
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
  useState(() => {
    const t = setTimeout(onClose, 3000)
    return () => clearTimeout(t)
  })

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

function hasInProgress(deploys: DeployResponse[]): boolean {
  return deploys.some((d) => d.in_progress)
}

export function DeploySection({
  binding,
  slotKind,
  slotProvider,
}: {
  binding: Binding
  slotKind: string
  slotProvider: string
}) {
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [deploying, setDeploying] = useState(false)
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error'; id: number } | null>(null)
  const [toastId, setToastId] = useState(0)
  const [logDeploy, setLogDeploy] = useState<DeployResponse | null>(null)
  const [logBindingId, setLogBindingId] = useState<string | null>(null)

  const showToast = useCallback((message: string, type: 'success' | 'error') => {
    setToastId((id) => id + 1)
    setToast({ message, type, id: toastId + 1 })
  }, [toastId])

  const {
    data: deploys,
    isLoading: deploysLoading,
    isError: deploysError,
    refetch: refetchDeploys,
  } = useQuery({
    queryKey: ['binding-deploys', binding.id],
    queryFn: () => listDeploys(binding.id),
    refetchInterval: (query) => {
      const data = query.state.data
      if (data && hasInProgress(data)) {
        return 5000
      }
      return false
    },
  })

  const triggerMutation = useMutation({
    mutationFn: () => triggerDeploy(binding.id),
    onSuccess: () => {
      setDeploying(false)
      setConfirmOpen(false)
      refetchDeploys()
    },
    onError: (error: Error) => {
      setDeploying(false)
      setConfirmOpen(false)
      showToast(error.message, 'error')
    },
  })

  const handleTrigger = useCallback(async () => {
    setDeploying(true)
    triggerMutation.mutate()
  }, [triggerMutation])

  const isCFDirectUpload = slotProvider === 'cloudflare' && slotKind === 'static-site'

  const deploysList: DeployResponse[] = deploys ?? []

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h4 className="text-sm font-medium">
          {binding.external_id}
        </h4>
        {isCFDirectUpload ? (
          <span
            className="cursor-not-allowed text-xs text-muted-foreground"
            title="Deploy via Direct Upload is unsupported. Use the wrangler CLI or dashboard."
          >
            <Button variant="outline" size="xs" disabled>
              Deploy
            </Button>
            <span className="ml-2">Unsupported</span>
          </span>
        ) : (
          <Button
            variant="outline"
            size="xs"
            disabled={deploying}
            onClick={() => setConfirmOpen(true)}
          >
            {deploying ? 'Deploying...' : 'Deploy'}
          </Button>
        )}
      </div>

      {deploysLoading && (
        <p className="text-xs text-muted-foreground">Loading deployments...</p>
      )}

      {deploysError && (
        <div className="flex items-center gap-2">
          <p className="text-xs text-destructive">Failed to load deployments.</p>
          <button className="text-xs text-primary underline" onClick={() => refetchDeploys()}>
            Retry
          </button>
        </div>
      )}

      {!deploysLoading && !deploysError && deploysList.length === 0 && (
        <p className="text-xs text-muted-foreground">No deployments yet.</p>
      )}

      {deploysList.length > 0 && (
        <div className="space-y-1">
          {deploysList.map((dep) => (
            <button
              key={dep.id}
              type="button"
              className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-xs transition-colors hover:bg-muted"
              onClick={() => { setLogDeploy(dep); setLogBindingId(binding.id) }}
            >
              <DeployStatusIcon status={dep.status} inProgress={dep.in_progress} />
              <span className="font-medium">
                {DEPLOY_STATUS_LABELS[dep.status] ?? dep.status}
              </span>
              <span className="text-muted-foreground">
                {relativeTime(dep.created_at)}
              </span>
            </button>
          ))}
        </div>
      )}

      <ConfirmDialog
        open={confirmOpen}
        title="Trigger Deployment"
        message={`Deploy ${binding.external_id} now?`}
        confirmLabel="Deploy"
        onConfirm={handleTrigger}
        onCancel={() => setConfirmOpen(false)}
      />

      {toast && (
        <Toast
          key={toast.id}
          message={toast.message}
          type={toast.type}
          onClose={() => setToast(null)}
        />
      )}

      {logDeploy && logBindingId && (
        <LogDrawer
          bindingId={logBindingId}
          deployId={logDeploy.id}
          open={!!logDeploy}
          onClose={() => { setLogDeploy(null); setLogBindingId(null) }}
        />
      )}
    </div>
  )
}