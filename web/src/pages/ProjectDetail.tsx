import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, Link2, Pencil, Plus, Unlink } from 'lucide-react'
import { Dialog } from 'radix-ui'
import { Link, useParams } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { apiFetch, type Catalog, type ProjectDetail as Detail, type ProjectResource, type ResourceInstance, type ResourceRole } from '@/lib/api'

type FormState = { instanceID: string; alias: string; role: ResourceRole | ''; purpose: string }
const emptyForm: FormState = { instanceID: '', alias: '', role: '', purpose: '' }

function ProjectDetail() {
  const { id } = useParams()
  const qc = useQueryClient()
  const [attachOpen, setAttachOpen] = useState(false)
  const [editing, setEditing] = useState<ProjectResource | null>(null)
  const [form, setForm] = useState<FormState>(emptyForm)
  const [error, setError] = useState('')
  const { data: detail, isLoading } = useQuery({ queryKey: ['project', id], queryFn: () => apiFetch<Detail>(`/projects/${id}`), enabled: Boolean(id) })
  const { data: inventory = [] } = useQuery({ queryKey: ['resources'], queryFn: () => apiFetch<ResourceInstance[]>('/resource-instances') })
  const { data: catalog } = useQuery({ queryKey: ['catalog'], queryFn: () => apiFetch<Catalog>('/catalog/providers') })
  const products = useMemo(() => new Map(catalog?.providers.flatMap(provider => provider.products).map(product => [product.id, product]) ?? []), [catalog])
  const roleLabels = useMemo(() => new Map(catalog?.resource_roles?.map(role => [role.id, role.display_name]) ?? []), [catalog])
  const attached = new Set((detail?.resources ?? []).map(item => item.resource_instance.id))
  const available = inventory.filter(item => !attached.has(item.id))
  const selectedResource = inventory.find(item => item.id === form.instanceID)
  const compatibleRoles = selectedResource ? (products.get(selectedResource.provider_product_id)?.compatible_roles ?? []) : []

  const attach = useMutation({
    mutationFn: () => apiFetch<ProjectResource>(`/projects/${id}/resources`, { method: 'POST', body: JSON.stringify({ resource_instance_id: form.instanceID, alias: form.alias, role: form.role, purpose: form.purpose }) }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['project', id] }); setAttachOpen(false); setForm(emptyForm); setError('') },
    onError: (value: Error) => setError(value.message),
  })
  const update = useMutation({
    mutationFn: () => apiFetch<ProjectResource>(`/projects/${id}/resources/${editing?.id}`, { method: 'PATCH', body: JSON.stringify({ alias: form.alias, role: form.role, purpose: form.purpose }) }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['project', id] }); setEditing(null); setForm(emptyForm); setError('') },
    onError: (value: Error) => setError(value.message),
  })
  const detach = useMutation({
    mutationFn: (linkID: string) => apiFetch<void>(`/projects/${id}/resources/${linkID}`, { method: 'DELETE' }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['project', id] }),
    onError: (value: Error) => setError(value.message),
  })

  if (isLoading) return <p className="text-sm text-muted-foreground">Loading…</p>
  if (!detail) return <p>Project not found.</p>
  const resources = detail.resources ?? []
  const relations = detail.relations ?? []
  const names = new Map(resources.map(item => [item.resource_instance.id, item.project_resource.alias]))

  function selectResource(instanceID: string) {
    const item = available.find(value => value.id === instanceID)
    const roles = item ? (products.get(item.provider_product_id)?.compatible_roles ?? []) : []
    setForm(current => ({ ...current, instanceID, alias: current.alias || item?.display_name || '', role: roles[0] ?? '' }))
  }
  function startEdit(link: ProjectResource) {
    setEditing(link)
    setForm({ instanceID: link.resource_instance_id, alias: link.alias, role: link.role, purpose: link.purpose ?? '' })
    setError('')
  }

  return <div>
    <Link to="/projects" className="mb-6 inline-flex items-center gap-1 text-sm text-muted-foreground"><ArrowLeft className="size-4" />Projects</Link>
    <div className="mb-8 flex items-end justify-between"><div><p className="text-xs uppercase tracking-[.18em] text-muted-foreground">Project</p><h1 className="mt-2 text-2xl font-medium">{detail.project.name}</h1><p className="mt-1 text-sm text-muted-foreground">{detail.project.description}</p></div><Button onClick={() => { setAttachOpen(true); setForm(emptyForm); setError('') }}><Plus className="size-4" />Attach resource</Button></div>
    {error && !attachOpen && !editing && <p className="mb-4 rounded-lg bg-destructive/10 p-3 text-sm text-destructive">{error}</p>}
    <div className="grid gap-3">
      {resources.map(({ project_resource: link, resource_instance: item }) => <div key={link.id} className="flex items-center gap-4 rounded-xl border p-4">
        <div className="flex size-9 items-center justify-center rounded-lg bg-muted text-xs uppercase">{item.resource_kind.slice(0, 2)}</div>
        <div className="flex-1"><div className="flex items-center gap-2"><Link to={`/inventory/${item.id}`} className="font-medium hover:underline">{link.alias}</Link><span className="rounded-full bg-muted px-2 py-0.5 text-[10px] uppercase tracking-wide text-muted-foreground">{roleLabels.get(link.role) ?? link.role}</span></div><p className="text-xs text-muted-foreground">{item.display_name} · {item.provider_product_id}{link.purpose ? ` · ${link.purpose}` : ''}</p></div>
        <span className="text-xs text-muted-foreground">{item.sync_status}</span><Button variant="ghost" size="icon" title="Edit project resource" onClick={() => startEdit(link)}><Pencil className="size-4" /></Button><Button variant="ghost" size="icon" title="Detach from project" onClick={() => detach.mutate(link.id)}><Unlink className="size-4" /></Button>
      </div>)}
      {resources.length === 0 && <div className="rounded-xl border p-12 text-center text-sm text-muted-foreground">Attach inventory resources to give this project its operational context.</div>}
    </div>
    <section className="mt-8"><div className="mb-3 flex items-center gap-2"><Link2 className="size-4" /><h2 className="font-medium">Resource relation subgraph</h2></div><div className="rounded-xl border p-5">{relations.length === 0 ? <p className="text-sm text-muted-foreground">No relations between attached resources.</p> : <div className="space-y-3">{relations.map(relation => <div key={relation.id} className="flex items-center gap-3 text-sm"><span className="rounded-md bg-muted px-2 py-1 font-medium">{names.get(relation.from_resource_instance_id)}</span><span className="text-xs text-muted-foreground">— {relation.relation_type} →</span><span className="rounded-md bg-muted px-2 py-1 font-medium">{names.get(relation.to_resource_instance_id)}</span><span className="text-[10px] uppercase text-muted-foreground">{relation.origin}</span></div>)}</div>}</div></section>
    <ResourceDialog open={attachOpen} title="Attach inventory resource" submitLabel="Attach" form={form} resources={available} roles={compatibleRoles} roleLabels={roleLabels} error={error} pending={attach.isPending} onOpenChange={setAttachOpen} onFormChange={setForm} onResourceChange={selectResource} onSubmit={() => attach.mutate()} />
    <ResourceDialog open={Boolean(editing)} title="Edit project resource" submitLabel="Save" form={form} resources={editing ? inventory.filter(item => item.id === editing.resource_instance_id) : []} roles={editing ? (products.get(inventory.find(item => item.id === editing.resource_instance_id)?.provider_product_id ?? '')?.compatible_roles ?? []) : []} roleLabels={roleLabels} error={error} pending={update.isPending} lockResource onOpenChange={open => { if (!open) setEditing(null) }} onFormChange={setForm} onResourceChange={() => {}} onSubmit={() => update.mutate()} />
  </div>
}

function ResourceDialog({ open, title, submitLabel, form, resources, roles, roleLabels, error, pending, lockResource = false, onOpenChange, onFormChange, onResourceChange, onSubmit }: {
  open: boolean; title: string; submitLabel: string; form: FormState; resources: ResourceInstance[]; roles: ResourceRole[]; roleLabels: Map<ResourceRole, string>; error: string; pending: boolean; lockResource?: boolean; onOpenChange: (open: boolean) => void; onFormChange: (form: FormState) => void; onResourceChange: (id: string) => void; onSubmit: () => void
}) {
  return <Dialog.Root open={open} onOpenChange={onOpenChange}><Dialog.Portal><Dialog.Overlay className="fixed inset-0 z-40 bg-black/25" /><Dialog.Content className="fixed left-1/2 top-1/2 z-50 w-[min(92vw,480px)] -translate-x-1/2 -translate-y-1/2 rounded-xl border bg-background p-6 shadow-xl"><Dialog.Title className="mb-5 text-lg font-medium">{title}</Dialog.Title><div className="space-y-4">
    <label className="block text-sm">Resource<select disabled={lockResource} className="mt-1 h-9 w-full rounded-md border bg-background px-3 disabled:opacity-60" value={form.instanceID} onChange={event => onResourceChange(event.target.value)}><option value="">Choose a resource</option>{resources.map(item => <option key={item.id} value={item.id}>{item.display_name} · {item.resource_kind}</option>)}</select></label>
    <label className="block text-sm">Project alias<Input className="mt-1" value={form.alias} onChange={event => onFormChange({ ...form, alias: event.target.value })} /></label>
    <label className="block text-sm">Role<select className="mt-1 h-9 w-full rounded-md border bg-background px-3" value={form.role} onChange={event => onFormChange({ ...form, role: event.target.value as ResourceRole })}><option value="">Choose a role</option>{roles.map(role => <option key={role} value={role}>{roleLabels.get(role) ?? role}</option>)}</select></label>
    <label className="block text-sm">Purpose<Input className="mt-1" value={form.purpose} onChange={event => onFormChange({ ...form, purpose: event.target.value })} placeholder="production, docs, public DNS…" /></label>
    {error && <p className="rounded-lg bg-destructive/10 p-3 text-sm text-destructive">{error}</p>}<p className="text-xs text-muted-foreground">Pipelines and Git-connected Pages require their source repository to be attached first.</p><div className="flex justify-end gap-2"><Dialog.Close asChild><Button variant="outline">Cancel</Button></Dialog.Close><Button disabled={!form.instanceID || !form.alias || !form.role || pending} onClick={onSubmit}>{submitLabel}</Button></div>
  </div></Dialog.Content></Dialog.Portal></Dialog.Root>
}

export default ProjectDetail
