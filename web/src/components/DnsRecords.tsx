import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { apiFetch, ApiError } from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  DialogTrigger,
} from "@/components/ui/dialog"
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogAction,
  AlertDialogCancel,
} from "@/components/ui/alert-dialog"
import { Alert, AlertTitle, AlertDescription } from "@/components/ui/alert"
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "@/components/ui/table"
import { Plus, Pencil, Trash2, AlertTriangle, Inbox } from "lucide-react"

export interface DNSRecord {
  id?: string
  type: string
  name: string
  content: string
  ttl?: number
  proxied?: boolean | null
  priority?: number | null
  zone_id?: string
}

interface DnsRecordsProps {
  bindingId: string
  provider: string
  verified?: boolean
}

type DNSAction = "add" | "edit" | "delete"
type DNSDialog = DNSAction | null

const RECORD_TYPES = ["A", "AAAA", "CNAME", "TXT", "MX", "NS", "SRV", "CAA", "HTTPS"]

function DnsRecords({ bindingId, provider, verified }: DnsRecordsProps) {
  const qc = useQueryClient()
  const isCF = provider === "cloudflare"
  const isVercelUnverified = provider === "vercel" && verified === false

  const [dialog, setDialog] = useState<DNSDialog>(null)
  const [editRecord, setEditRecord] = useState<DNSRecord | null>(null)
  const [deleteRecordId, setDeleteRecordId] = useState<string | null>(null)
  const [deleteRecordName, setDeleteRecordName] = useState<string>("")

  const [formType, setFormType] = useState("A")
  const [formName, setFormName] = useState("")
  const [formContent, setFormContent] = useState("")
  const [formTTL, setFormTTL] = useState(3600)
  const [formProxied, setFormProxied] = useState(true)

  const recordsQuery = useQuery<DNSRecord[]>({
    queryKey: ["dns-records", bindingId],
    queryFn: () => apiFetch<DNSRecord[]>(`/bindings/${bindingId}/dns-records`),
  })

  const createMutation = useMutation({
    mutationFn: (record: DNSRecord) =>
      apiFetch<DNSRecord>(`/bindings/${bindingId}/dns-records`, {
        method: "POST",
        body: JSON.stringify(record),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["dns-records", bindingId] })
      closeAddDialog()
    },
  })

  const updateMutation = useMutation({
    mutationFn: ({ id, record }: { id: string; record: Partial<DNSRecord> }) =>
      apiFetch<DNSRecord>(`/bindings/${bindingId}/dns-records/${id}`, {
        method: "PATCH",
        body: JSON.stringify(record),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["dns-records", bindingId] })
      setDialog(null)
      setEditRecord(null)
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) =>
      apiFetch<void>(`/bindings/${bindingId}/dns-records/${id}`, {
        method: "DELETE",
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["dns-records", bindingId] })
      setDeleteRecordId(null)
      setDeleteRecordName("")
    },
  })

  function openAddDialog() {
    setFormType("A")
    setFormName("")
    setFormContent("")
    setFormTTL(3600)
    setFormProxied(true)
    setDialog("add")
  }

  function closeAddDialog() {
    setDialog(null)
  }

  function openEditDialog(rec: DNSRecord) {
    setEditRecord(rec)
    setFormType(rec.type)
    setFormName(rec.name)
    setFormContent(rec.content)
    setFormTTL(rec.ttl ?? 3600)
    setFormProxied(rec.proxied ?? true)
    setDialog("edit")
  }

  function handleCreate() {
    if (!formName || !formContent) return
    const record: DNSRecord = {
      type: formType,
      name: formName,
      content: formContent,
      ttl: formTTL,
    }
    if (isCF) {
      record.proxied = formProxied
    }
    createMutation.mutate(record)
  }

  function handleUpdate() {
    if (!editRecord?.id || !formContent) return
    const patch: Partial<DNSRecord> = {
      content: formContent,
      ttl: formTTL,
    }
    if (isCF) {
      patch.proxied = formProxied
    }
    updateMutation.mutate({ id: editRecord.id, record: patch })
  }

  function handleDelete(id: string) {
    deleteMutation.mutate(id)
  }

  if (recordsQuery.isPending) {
    return (
      <div className="flex items-center justify-center py-12">
        <p className="text-sm text-muted-foreground">Loading records...</p>
      </div>
    )
  }

  if (recordsQuery.isError) {
    const msg = recordsQuery.error instanceof ApiError
      ? recordsQuery.error.message
      : "Failed to load DNS records"
    return (
      <Alert variant="destructive">
        <AlertTriangle className="size-4" />
        <AlertTitle>Error</AlertTitle>
        <AlertDescription>{msg}</AlertDescription>
      </Alert>
    )
  }

  const records = recordsQuery.data ?? []

  return (
    <div>
      <div className="flex items-center justify-between mb-4">
        <h3 className="font-heading text-base font-medium">DNS Records</h3>
        <Dialog open={dialog === "add"} onOpenChange={(open) => !open && closeAddDialog()}>
          <DialogTrigger asChild>
            <Button variant="outline" size="sm" onClick={openAddDialog}>
              <Plus className="size-3.5" />
              Add Record
            </Button>
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Add DNS Record</DialogTitle>
              <DialogDescription>Create a new DNS record for this domain.</DialogDescription>
            </DialogHeader>
            <div className="grid gap-3">
              <div className="grid gap-1.5">
                <Label>Type</Label>
                <Select value={formType} onValueChange={setFormType}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {RECORD_TYPES.map((t) => (
                      <SelectItem key={t} value={t}>{t}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="grid gap-1.5">
                <Label>Name</Label>
                <Input
                  placeholder="www"
                  value={formName}
                  onChange={(e) => setFormName(e.target.value)}
                />
              </div>
              <div className="grid gap-1.5">
                <Label>Content</Label>
                <Input
                  placeholder={isCF ? "192.0.2.1" : "value"}
                  value={formContent}
                  onChange={(e) => setFormContent(e.target.value)}
                />
              </div>
              <div className="grid gap-1.5">
                <Label>TTL (seconds)</Label>
                <Input
                  type="number"
                  min={60}
                  max={86400}
                  value={formTTL}
                  onChange={(e) => setFormTTL(Number(e.target.value))}
                />
              </div>
              {isCF && (
                <div className="flex items-center justify-between">
                  <Label>Proxied</Label>
                  <Switch
                    checked={formProxied}
                    onCheckedChange={setFormProxied}
                  />
                </div>
              )}
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={closeAddDialog}>Cancel</Button>
              <Button onClick={handleCreate} disabled={!formName || !formContent || createMutation.isPending}>
                {createMutation.isPending ? "Creating..." : "Create"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>

      {isVercelUnverified && (
        <Alert variant="warning" className="mb-4">
          <AlertTriangle className="size-4" />
          <AlertTitle>Domain Not Verified</AlertTitle>
          <AlertDescription>
            This domain has not been verified with Vercel. DNS records may not propagate until verification is complete.
          </AlertDescription>
        </Alert>
      )}

      {records.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-16 text-center">
          <Inbox className="size-10 text-muted-foreground/50 mb-3" />
          <p className="text-sm font-medium text-muted-foreground">No DNS records</p>
          <p className="text-xs text-muted-foreground/60 mt-1">Add a record to get started.</p>
        </div>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-20">Type</TableHead>
              <TableHead>Name</TableHead>
              <TableHead>{isCF ? "Content" : "Value"}</TableHead>
              <TableHead className="w-20">TTL</TableHead>
              {isCF && <TableHead className="w-20">Proxied</TableHead>}
              <TableHead className="w-24 text-right">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {records.map((rec) => (
              <TableRow key={rec.id ?? rec.name + rec.type}>
                <TableCell className="font-mono text-xs">{rec.type}</TableCell>
                <TableCell className="max-w-[200px] truncate font-medium">
                  {rec.name}
                </TableCell>
                <TableCell className="max-w-[250px] truncate font-mono text-xs text-muted-foreground">
                  {rec.content}
                </TableCell>
                <TableCell className="font-mono text-xs">{rec.ttl ?? "-"}</TableCell>
                {isCF && (
                  <TableCell>
                    {rec.proxied === true ? (
                      <span className="text-xs font-medium text-blue-600 bg-blue-50 px-1.5 py-0.5 rounded">Yes</span>
                    ) : rec.proxied === false ? (
                      <span className="text-xs text-muted-foreground">No</span>
                    ) : (
                      <span className="text-xs text-muted-foreground/50">-</span>
                    )}
                  </TableCell>
                )}
                <TableCell className="text-right">
                  <div className="flex items-center justify-end gap-1">
                    <Dialog
                      open={dialog === "edit" && editRecord?.id === rec.id}
                      onOpenChange={(open) => {
                        if (!open) { setDialog(null); setEditRecord(null) }
                      }}
                    >
                      <DialogTrigger asChild>
                        <Button
                          variant="ghost"
                          size="icon-xs"
                          onClick={() => openEditDialog(rec)}
                        >
                          <Pencil className="size-3" />
                        </Button>
                      </DialogTrigger>
                      <DialogContent>
                        <DialogHeader>
                          <DialogTitle>Edit Record</DialogTitle>
                          <DialogDescription>
                            {rec.type} {rec.name}
                          </DialogDescription>
                        </DialogHeader>
                        <div className="grid gap-3">
                          <div className="grid gap-1.5">
                            <Label>Content</Label>
                            <Input
                              value={formContent}
                              onChange={(e) => setFormContent(e.target.value)}
                            />
                          </div>
                          <div className="grid gap-1.5">
                            <Label>TTL (seconds)</Label>
                            <Input
                              type="number"
                              min={60}
                              max={86400}
                              value={formTTL}
                              onChange={(e) => setFormTTL(Number(e.target.value))}
                            />
                          </div>
                          {isCF && (
                            <div className="flex items-center justify-between">
                              <Label>Proxied</Label>
                              <Switch
                                checked={formProxied}
                                onCheckedChange={setFormProxied}
                              />
                            </div>
                          )}
                        </div>
                        <DialogFooter>
                          <Button variant="outline" onClick={() => { setDialog(null); setEditRecord(null) }}>
                            Cancel
                          </Button>
                          <Button onClick={handleUpdate} disabled={!formContent || updateMutation.isPending}>
                            {updateMutation.isPending ? "Saving..." : "Save"}
                          </Button>
                        </DialogFooter>
                      </DialogContent>
                    </Dialog>

                    <AlertDialog
                      open={deleteRecordId === rec.id}
                      onOpenChange={(open) => {
                        if (!open) { setDeleteRecordId(null); setDeleteRecordName("") }
                      }}
                    >
                      <AlertDialogTrigger asChild>
                        <Button
                          variant="ghost"
                          size="icon-xs"
                          onClick={() => {
                            setDeleteRecordId(rec.id ?? null)
                            setDeleteRecordName(rec.name)
                          }}
                        >
                          <Trash2 className="size-3 text-destructive" />
                        </Button>
                      </AlertDialogTrigger>
                      <AlertDialogContent>
                        <AlertDialogHeader>
                          <AlertDialogTitle>Delete Record</AlertDialogTitle>
                          <AlertDialogDescription>
                            Are you sure you want to delete the {rec.type} record for "{rec.name}"? This action cannot be undone.
                          </AlertDialogDescription>
                        </AlertDialogHeader>
                        <AlertDialogFooter>
                          <AlertDialogCancel>Cancel</AlertDialogCancel>
                          <AlertDialogAction
                            className="bg-destructive text-destructive-foreground hover:bg-destructive/80"
                            onClick={() => rec.id && handleDelete(rec.id)}
                          >
                            Delete
                          </AlertDialogAction>
                        </AlertDialogFooter>
                      </AlertDialogContent>
                    </AlertDialog>
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}

      {createMutation.isError && (
        <Alert variant="destructive" className="mt-3">
          <AlertTriangle className="size-4" />
          <AlertTitle>Failed to create record</AlertTitle>
          <AlertDescription>
            {createMutation.error instanceof ApiError
              ? createMutation.error.message
              : "Unknown error"}
          </AlertDescription>
        </Alert>
      )}

      {updateMutation.isError && (
        <Alert variant="destructive" className="mt-3">
          <AlertTriangle className="size-4" />
          <AlertTitle>Failed to update record</AlertTitle>
          <AlertDescription>
            {updateMutation.error instanceof ApiError
              ? updateMutation.error.message
              : "Unknown error"}
          </AlertDescription>
        </Alert>
      )}

      {deleteMutation.isError && (
        <Alert variant="destructive" className="mt-3">
          <AlertTriangle className="size-4" />
          <AlertTitle>Failed to delete record</AlertTitle>
          <AlertDescription>
            {deleteMutation.error instanceof ApiError
              ? deleteMutation.error.message
              : "Unknown error"}
          </AlertDescription>
        </Alert>
      )}
    </div>
  )
}

export { DnsRecords }