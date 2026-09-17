import { useState } from "react"
import { fetchDeployLogs } from "@/lib/api"
import { Button } from "@/components/ui/button"

interface LogDrawerProps {
  bindingId: string
  deployId: string
  open: boolean
  onClose: () => void
}

function LogDrawer({ bindingId, deployId, open, onClose }: LogDrawerProps) {
  const [logs, setLogs] = useState("")

  useState(() => {
    if (open && deployId) {
      fetchDeployLogs(bindingId, deployId).then((r) => setLogs(r.lines)).catch(() => setLogs("failed to fetch logs"))
    }
  })

  if (!open) return null

  return (
    <div className="fixed inset-0 z-50 flex items-end bg-black/20" onClick={onClose}>
      <div className="w-full max-h-96 rounded-t-xl bg-background p-4 ring-1 ring-foreground/10" onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center justify-between mb-2">
          <h3 className="text-sm font-medium">Deploy Logs</h3>
          <Button variant="outline" size="xs" onClick={onClose}>Close</Button>
        </div>
        <pre className="max-h-72 overflow-auto rounded-lg bg-muted p-4 text-xs font-mono leading-relaxed whitespace-pre-wrap">
          {logs || "no logs"}
        </pre>
      </div>
    </div>
  )
}

export { LogDrawer }
