import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'
import { apiFetch, ApiError } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import { Dialog } from 'radix-ui'
import {
  ArrowLeft,
  Plus,
  Code,
  Cpu,
  Globe,
  Globe2,
  AlertCircle,
} from 'lucide-react'

interface Project {
  id: string
  name: string
  description?: string
  created_at: string
  updated_at: string
}

interface Slot {
  id: string
  project_id: string
  name: string
  kind: 'repo' | 'compute' | 'static-site' | 'dns-domain'
  provider: string
  config?: Record<string, unknown>
  created_at: string
}

interface Binding {
  id: string
  slot_id: string
  account_id: string
  external_id: string
  external_url?: string
  cached_meta?: Record<string, unknown>
  sync_status: 'ok' | 'error' | 'auth_error' | 'orphaned' | 'never'
  last_synced_at?: string
  created_at: string
}

interface ProjectDetail {
  project: Project
  slots: Slot[]
  bindings: Binding[]
}

const slotTypeIcons: Record<string, typeof Code> = {
  repo: Code,
  compute: Cpu,
  'static-site': Globe,
  'dns-domain': Globe2,
}

const slotTypeLabels: Record<string, string> = {
  repo: 'Repo',
  compute: 'Compute',
  'static-site': 'Static Site',
  'dns-domain': 'DNS Domain',
}

const statusColors: Record<string, string> = {
  ok: 'text-emerald-500',
  error: 'text-red-500',
  auth_error: 'text-amber-500',
  orphaned: 'text-muted-foreground/40',
  never: 'text-muted-foreground/40',
}

function StatusDot({ status }: { status: string }) {
  return (
    <span
      className={cn('inline-block size-2 rounded-full', statusColors[status] || 'text-muted-foreground/40')}
      style={{ backgroundColor: 'currentColor' }}
    />
  )
}

function formatDate(iso: string) {
  return new Date(iso).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })
}

function ProjectDetail() {
  const { id } = useParams<{ id: string }>()
  const qc = useQueryClient()
  const [addOpen, setAddOpen] = useState(false)
  const [slotType, setSlotType] = useState('repo')
  const [slotName, setSlotName] = useState('')
  const [slotPrivate, setSlotPrivate] = useState(false)
  const [slotDescription, setSlotDescription] = useState('')
  const [slotWorkflowId, setSlotWorkflowId] = useState('')
  const [slotWorkflowRef, setSlotWorkflowRef] = useState('')
  const [slotCompatibilityDate, setSlotCompatibilityDate] = useState('')
  const [slotFramework, setSlotFramework] = useState('')
  const [slotBuildCommand, setSlotBuildCommand] = useState('')
  const [slotOutputDir, setSlotOutputDir] = useState('')
  const [slotProductionBranch, setSlotProductionBranch] = useState('')
  const [formErrors, setFormErrors] = useState<Record<string, string>>({})

  const { data: detail, isLoading } = useQuery<ProjectDetail>({
    queryKey: ['project', id],
    queryFn: () => apiFetch<ProjectDetail>(`/projects/${id}`),
    enabled: Boolean(id),
  })

  const createSlotMut = useMutation({
    mutationFn: (body: { type: string; name: string; config: Record<string, unknown> }) =>
      apiFetch<Slot>(`/projects/${id}/slots`, {
        method: 'POST',
        body: JSON.stringify(body),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['project', id] })
      resetForm()
      setAddOpen(false)
    },
    onError: (err: ApiError) => {
      const msg = err.message.toLowerCase()
      const errors: Record<string, string> = {}

      if (msg.includes('name is required') || msg.includes('name')) {
        errors.name = 'Name is required'
      }

      setFormErrors(errors)
    },
  })

  function resetForm() {
    setSlotType('repo')
    setSlotName('')
    setSlotPrivate(false)
    setSlotDescription('')
    setSlotWorkflowId('')
    setSlotWorkflowRef('')
    setSlotCompatibilityDate('')
    setSlotFramework('')
    setSlotBuildCommand('')
    setSlotOutputDir('')
    setSlotProductionBranch('')
    setFormErrors({})
  }

  function buildConfig(): Record<string, unknown> {
    const base: Record<string, unknown> = { name: slotName.trim() }
    switch (slotType) {
      case 'repo':
        base.private = slotPrivate
        if (slotDescription.trim()) base.description = slotDescription.trim()
        if (slotWorkflowId.trim()) base.workflow_id = slotWorkflowId.trim()
        if (slotWorkflowRef.trim()) base.workflow_ref = slotWorkflowRef.trim()
        break
      case 'compute':
        if (slotCompatibilityDate.trim()) base.compatibility_date = slotCompatibilityDate.trim()
        break
      case 'static-site':
        if (slotFramework.trim()) base.framework = slotFramework.trim()
        if (slotBuildCommand.trim()) base.build_command = slotBuildCommand.trim()
        if (slotOutputDir.trim()) base.output_dir = slotOutputDir.trim()
        if (slotProductionBranch.trim()) base.production_branch = slotProductionBranch.trim()
        break
      case 'dns-domain':
        break
    }
    return base
  }

  function handleAddSlot() {
    const errors: Record<string, string> = {}
    if (!slotName.trim()) errors.name = 'Name is required'
    setFormErrors(errors)
    if (Object.keys(errors).length > 0) return

    createSlotMut.mutate({ type: slotType, name: slotName.trim(), config: buildConfig() })
  }

  function handleDialogOpenChange(open: boolean) {
    setAddOpen(open)
    if (!open) resetForm()
  }

  if (isLoading) {
    return <p className="text-sm text-muted-foreground">Loading...</p>
  }

  if (!detail) {
    return <p className="text-sm text-muted-foreground">Project not found.</p>
  }

  const { project, slots, bindings } = detail
  const bindingsBySlot: Record<string, Binding[]> = {}
  for (const b of bindings) {
    if (!bindingsBySlot[b.slot_id]) bindingsBySlot[b.slot_id] = []
    bindingsBySlot[b.slot_id].push(b)
  }

  const statusPriority: Record<string, number> = {
    ok: 0,
    never: 1,
    orphaned: 2,
    error: 3,
    auth_error: 4,
  }

  function worstStatus(bindings: Binding[]): string {
    let worst = 'ok'
    for (const b of bindings) {
      if ((statusPriority[b.sync_status] ?? 0) > (statusPriority[worst] ?? 0)) {
        worst = b.sync_status
      }
    }
    return worst
  }

  return (
    <div>
      <Link
        to="/projects"
        className="mb-6 inline-flex items-center gap-1 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground"
      >
        <ArrowLeft className="size-3" />
        Back to Projects
      </Link>

      <div className="mb-8">
        <h1 className="font-heading text-2xl font-medium tracking-tight">
          {project.name}
        </h1>
        {project.description && (
          <p className="mt-1 text-sm text-muted-foreground">
            {project.description}
          </p>
        )}
        <p className="mt-2 text-xs text-muted-foreground/60">
          Created {formatDate(project.created_at)} &middot;{' '}
          {slots.length} slot{slots.length !== 1 ? 's' : ''}
        </p>
      </div>

      <div className="mb-4 flex items-center justify-between">
        <h2 className="font-heading text-base font-medium tracking-tight">
          Slots
        </h2>
        <Button onClick={() => setAddOpen(true)}>
          <Plus className="size-4" />
          Add Slot
        </Button>
      </div>

      {slots.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-xl border border-border py-16 text-center">
          <Plus className="mb-3 size-8 text-muted-foreground/30" />
          <p className="text-sm font-medium text-muted-foreground">
            No slots yet
          </p>
          <p className="mt-1 text-xs text-muted-foreground/60">
            Add a slot to connect your first resource.
          </p>
        </div>
      ) : (
        <div className="flex flex-col divide-y divide-border rounded-xl border border-border">
          {slots.map((slot) => {
            const slotBindings = bindingsBySlot[slot.id] || []
            const Icon = slotTypeIcons[slot.kind] || Code
            return (
              <Link
                key={slot.id}
                to={`/projects/${slot.project_id}/slots/${slot.id}`}
                className="flex items-center gap-3 px-5 py-3.5 transition-colors hover:bg-muted/50"
              >
                <div className="flex size-8 shrink-0 items-center justify-center rounded-lg border border-border bg-card">
                  <Icon className="size-4 text-muted-foreground" />
                </div>
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="text-xs font-medium uppercase tracking-wider text-muted-foreground/60">
                      {slotTypeLabels[slot.kind] || slot.kind}
                    </span>
                    <StatusDot status={slotBindings.length > 0 ? worstStatus(slotBindings) : 'never'} />
                  </div>
                  <p className="truncate text-sm font-medium">{slot.name}</p>
                </div>
                <span className="shrink-0 text-xs text-muted-foreground/60">
                  {slotBindings.length} binding{slotBindings.length !== 1 ? 's' : ''}
                </span>
              </Link>
            )
          })}
        </div>
      )}

      <Dialog.Root open={addOpen} onOpenChange={handleDialogOpenChange}>
        <Dialog.Portal>
          <Dialog.Overlay className="fixed inset-0 z-40 bg-black/20 backdrop-blur-sm data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0" />
          <Dialog.Content className="fixed left-1/2 top-1/2 z-50 max-h-[85vh] w-full max-w-md -translate-x-1/2 -translate-y-1/2 overflow-y-auto rounded-xl border border-border bg-background p-6 shadow-lg data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95">
            <Dialog.Title className="font-heading text-lg font-medium tracking-tight">
              Add Slot
            </Dialog.Title>

            <div className="mt-5 flex flex-col gap-4">
              <div className="flex flex-col gap-1.5">
                <label className="text-xs font-medium text-foreground">
                  Type
                </label>
                <select
                  value={slotType}
                  onChange={(e) => { setSlotType(e.target.value); setFormErrors({}) }}
                  className="h-8 w-full rounded-lg border border-input bg-transparent px-2.5 py-1 text-sm transition-colors focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
                >
                  <option value="repo">Repo</option>
                  <option value="compute">Compute</option>
                  <option value="static-site">Static Site</option>
                  <option value="dns-domain">DNS Domain</option>
                </select>
              </div>

              <div className="flex flex-col gap-1.5">
                <label className="text-xs font-medium text-foreground">
                  Name
                </label>
                <Input
                  value={slotName}
                  onChange={(e) => setSlotName(e.target.value)}
                  placeholder="my-slot"
                  aria-invalid={Boolean(formErrors.name)}
                />
                {formErrors.name && (
                  <p className="flex items-center gap-1 text-xs text-destructive">
                    <AlertCircle className="size-3" />
                    {formErrors.name}
                  </p>
                )}
              </div>

              {slotType === 'repo' && (
                <>
                  <div className="flex items-center gap-3">
                    <label className="text-xs font-medium text-foreground">
                      Private
                    </label>
                    <button
                      type="button"
                      role="switch"
                      aria-checked={slotPrivate}
                      onClick={() => setSlotPrivate(!slotPrivate)}
                      className={cn(
                        'relative inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full border-2 border-transparent transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2',
                        slotPrivate ? 'bg-foreground' : 'bg-input',
                      )}
                    >
                      <span
                        className={cn(
                          'pointer-events-none block size-4 rounded-full bg-background shadow-sm ring-0 transition-transform',
                          slotPrivate ? 'translate-x-4' : 'translate-x-0',
                        )}
                      />
                    </button>
                  </div>
                  <div className="flex flex-col gap-1.5">
                    <label className="text-xs font-medium text-foreground">
                      Description
                    </label>
                    <Input
                      value={slotDescription}
                      onChange={(e) => setSlotDescription(e.target.value)}
                      placeholder="Optional description"
                    />
                  </div>
                  <div className="flex flex-col gap-1.5">
                    <label className="text-xs font-medium text-foreground">
                      Workflow ID
                    </label>
                    <Input
                      value={slotWorkflowId}
                      onChange={(e) => setSlotWorkflowId(e.target.value)}
                      placeholder="e.g. deploy.yml"
                    />
                  </div>
                  <div className="flex flex-col gap-1.5">
                    <label className="text-xs font-medium text-foreground">
                      Workflow Ref
                    </label>
                    <Input
                      value={slotWorkflowRef}
                      onChange={(e) => setSlotWorkflowRef(e.target.value)}
                      placeholder="e.g. main"
                    />
                  </div>
                </>
              )}

              {slotType === 'compute' && (
                <div className="flex flex-col gap-1.5">
                  <label className="text-xs font-medium text-foreground">
                    Compatibility Date
                  </label>
                  <Input
                    value={slotCompatibilityDate}
                    onChange={(e) => setSlotCompatibilityDate(e.target.value)}
                    placeholder="e.g. 2025-01-01"
                  />
                </div>
              )}

              {slotType === 'static-site' && (
                <>
                  <div className="flex flex-col gap-1.5">
                    <label className="text-xs font-medium text-foreground">
                      Framework
                    </label>
                    <Input
                      value={slotFramework}
                      onChange={(e) => setSlotFramework(e.target.value)}
                      placeholder="e.g. next"
                    />
                  </div>
                  <div className="flex flex-col gap-1.5">
                    <label className="text-xs font-medium text-foreground">
                      Build Command
                    </label>
                    <Input
                      value={slotBuildCommand}
                      onChange={(e) => setSlotBuildCommand(e.target.value)}
                      placeholder="e.g. npm run build"
                    />
                  </div>
                  <div className="flex flex-col gap-1.5">
                    <label className="text-xs font-medium text-foreground">
                      Output Directory
                    </label>
                    <Input
                      value={slotOutputDir}
                      onChange={(e) => setSlotOutputDir(e.target.value)}
                      placeholder="e.g. .next"
                    />
                  </div>
                  <div className="flex flex-col gap-1.5">
                    <label className="text-xs font-medium text-foreground">
                      Production Branch
                    </label>
                    <Input
                      value={slotProductionBranch}
                      onChange={(e) => setSlotProductionBranch(e.target.value)}
                      placeholder="e.g. main"
                    />
                  </div>
                </>
              )}

              {slotType === 'dns-domain' && (
                <p className="rounded-lg border border-border bg-muted/50 px-3 py-2 text-xs leading-relaxed text-muted-foreground">
                  DNS domain slots are bind-only. Attach an existing Cloudflare zone or domain
                  through a binding — no additional configuration is needed here.
                </p>
              )}

              {formErrors.name == null && createSlotMut.error && (
                <p className="flex items-center gap-1 text-xs text-destructive">
                  <AlertCircle className="size-3" />
                  {(createSlotMut.error as ApiError).message}
                </p>
              )}

              <div className="mt-2 flex justify-end gap-2">
                <Dialog.Close asChild>
                  <Button variant="outline">Cancel</Button>
                </Dialog.Close>
                <Button onClick={handleAddSlot} disabled={createSlotMut.isPending}>
                  {createSlotMut.isPending ? 'Adding...' : 'Add Slot'}
                </Button>
              </div>
            </div>
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>
    </div>
  )
}

export default ProjectDetail