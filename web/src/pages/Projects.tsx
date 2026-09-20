import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { apiFetch, ApiError } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Dialog } from 'radix-ui'
import { Plus, Trash2, FolderKanban, AlertCircle } from 'lucide-react'

interface Project {
  id: string
  name: string
  description?: string
  created_at: string
  updated_at: string
}

function formatDate(iso: string) {
  return new Date(iso).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })
}

function Projects() {
  const qc = useQueryClient()
  const [newOpen, setNewOpen] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<Project | null>(null)
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [formError, setFormError] = useState('')

  const { data: projects = [], isLoading } = useQuery<Project[]>({
    queryKey: ['projects'],
    queryFn: () => apiFetch<Project[]>('/projects'),
  })

  const createMut = useMutation({
    mutationFn: (body: { name: string; description: string }) =>
      apiFetch<Project>('/projects', {
        method: 'POST',
        body: JSON.stringify(body),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['projects'] })
      setNewOpen(false)
      setName('')
      setDescription('')
      setFormError('')
    },
    onError: (err: ApiError) => {
      setFormError(err.message)
    },
  })

  const deleteMut = useMutation({
    mutationFn: (id: string) =>
      apiFetch<void>(`/projects/${id}`, { method: 'DELETE' }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['projects'] })
      setDeleteTarget(null)
    },
  })

  function handleCreate() {
    if (!name.trim()) {
      setFormError('name is required')
      return
    }
    createMut.mutate({ name: name.trim(), description: description.trim() })
  }

  return (
    <div>
      <div className="mb-8 flex items-center justify-between">
        <div>
          <h1 className="font-heading text-2xl font-medium tracking-tight">
            Projects
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {projects.length} project{projects.length !== 1 ? 's' : ''}
          </p>
        </div>
        <Button onClick={() => setNewOpen(true)}>
          <Plus className="size-4" />
          New Project
        </Button>
      </div>

      {isLoading ? (
        <p className="text-sm text-muted-foreground">Loading...</p>
      ) : projects.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-xl border border-border py-24 text-center">
          <FolderKanban className="mb-3 size-8 text-muted-foreground/50" />
          <p className="text-sm font-medium text-muted-foreground">
            No projects yet
          </p>
          <p className="mt-1 text-xs text-muted-foreground/60">
            Create your first project to get started.
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {projects.map((p) => (
            <Link
              key={p.id}
              to={`/projects/${p.id}`}
              className="group/card block"
            >
              <div className="flex h-full flex-col justify-between rounded-xl border border-border bg-card p-5 transition-[border-color,box-shadow] duration-200 hover:border-foreground/20 hover:shadow-[0_2px_8px_rgba(0,0,0,0.04)]">
                <div>
                  <div className="flex items-start justify-between gap-2">
                    <h3 className="font-heading text-base font-medium leading-snug">
                      {p.name}
                    </h3>
                  </div>
                  {p.description && (
                    <p className="mt-1.5 text-sm leading-relaxed text-muted-foreground line-clamp-2">
                      {p.description}
                    </p>
                  )}
                </div>
                <div className="mt-4 flex items-center justify-between text-xs text-muted-foreground/60">
                  <span>{formatDate(p.created_at)}</span>
                  <button
                    onClick={(e) => {
                      e.preventDefault()
                      e.stopPropagation()
                      setDeleteTarget(p)
                    }}
                    className="flex items-center gap-1 rounded-md px-1.5 py-0.5 text-muted-foreground/40 transition-colors hover:bg-destructive/10 hover:text-destructive"
                  >
                    <Trash2 className="size-3" />
                    Delete
                  </button>
                </div>
              </div>
            </Link>
          ))}
        </div>
      )}

      <Dialog.Root open={newOpen} onOpenChange={setNewOpen}>
        <Dialog.Portal>
          <Dialog.Overlay className="fixed inset-0 z-40 bg-black/20 backdrop-blur-sm data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0" />
          <Dialog.Content className="fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-xl border border-border bg-background p-6 shadow-lg data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95">
            <Dialog.Title className="font-heading text-lg font-medium tracking-tight">
              New Project
            </Dialog.Title>
            <div className="mt-5 flex flex-col gap-4">
              <div className="flex flex-col gap-1.5">
                <label className="text-xs font-medium text-foreground">
                  Name
                </label>
                <Input
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="my-project"
                  aria-invalid={formError === 'name is required'}
                />
                {formError === 'name is required' && (
                  <p className="flex items-center gap-1 text-xs text-destructive">
                    <AlertCircle className="size-3" />
                    Name is required
                  </p>
                )}
              </div>
              <div className="flex flex-col gap-1.5">
                <label className="text-xs font-medium text-foreground">
                  Description
                </label>
                <Input
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  placeholder="Optional description"
                />
              </div>
              {formError && formError !== 'name is required' && (
                <p className="flex items-center gap-1 text-xs text-destructive">
                  <AlertCircle className="size-3" />
                  {formError}
                </p>
              )}
              <div className="mt-2 flex justify-end gap-2">
                <Dialog.Close asChild>
                  <Button variant="outline" onClick={() => { setName(''); setDescription(''); setFormError('') }}>
                    Cancel
                  </Button>
                </Dialog.Close>
                <Button onClick={handleCreate} disabled={createMut.isPending}>
                  {createMut.isPending ? 'Creating...' : 'Create'}
                </Button>
              </div>
            </div>
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>

      <Dialog.Root open={deleteTarget !== null} onOpenChange={(o) => { if (!o) setDeleteTarget(null) }}>
        <Dialog.Portal>
          <Dialog.Overlay className="fixed inset-0 z-40 bg-black/20 backdrop-blur-sm data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0" />
          <Dialog.Content className="fixed left-1/2 top-1/2 z-50 w-full max-w-sm -translate-x-1/2 -translate-y-1/2 rounded-xl border border-border bg-background p-6 shadow-lg data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95">
            <Dialog.Title className="font-heading text-lg font-medium tracking-tight">
              Delete Project
            </Dialog.Title>
            <p className="mt-2 text-sm text-muted-foreground">
              Delete <strong>{deleteTarget?.name}</strong>? Its project links will be detached, while global resource instances and remote resources remain untouched.
            </p>
            <div className="mt-5 flex justify-end gap-2">
              <Dialog.Close asChild>
                <Button variant="outline">Cancel</Button>
              </Dialog.Close>
              <Button
                variant="destructive"
                onClick={() => deleteTarget && deleteMut.mutate(deleteTarget.id)}
                disabled={deleteMut.isPending}
              >
                {deleteMut.isPending ? 'Deleting...' : 'Delete'}
              </Button>
            </div>
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>
    </div>
  )
}

export default Projects
