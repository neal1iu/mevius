import { useState, type FormEvent } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { setToken } from '@/lib/api'

interface GateProps {
  onEnter: () => void
}

function Gate({ onEnter }: GateProps) {
  const [token, setTokenValue] = useState('')

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const value = token.trim()
    if (!value) return
    setToken(value)
    onEnter()
  }

  return (
    <div className="flex min-h-svh items-center justify-center bg-background px-6">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle>mevius</CardTitle>
          <CardDescription>
            Enter your API token to access the control plane.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="flex flex-col gap-4">
            <Input
              type="password"
              placeholder="API token"
              value={token}
              onChange={(event) => setTokenValue(event.target.value)}
              autoFocus
            />
            <Button type="submit" className="w-full">
              Save
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}

export default Gate
