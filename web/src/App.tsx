import { useEffect, useState } from 'react'
import { QueryClient, QueryClientProvider, useQueryClient } from '@tanstack/react-query'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import AppLayout from '@/components/AppLayout'
import { getToken, setUnauthorizedHandler } from '@/lib/api'
import Accounts from '@/pages/Accounts'
import Gate from '@/pages/Gate'
import ProjectDetail from '@/pages/ProjectDetail'
import Projects from '@/pages/Projects'
import Inventory from '@/pages/Inventory'
import ResourceDetail from '@/pages/ResourceDetail'

const queryClient = new QueryClient()

function Root() {
  const [hasToken, setHasToken] = useState(() => Boolean(getToken()))
  const qc = useQueryClient()

  useEffect(() => {
    setUnauthorizedHandler(() => {
      qc.clear()
      setHasToken(false)
    })
    return () => setUnauthorizedHandler(null)
  }, [qc])

  if (!hasToken) {
    return (
      <Gate
        onEnter={() => {
          qc.clear()
          setHasToken(true)
        }}
      />
    )
  }

  return (
    <BrowserRouter>
      <Routes>
        <Route element={<AppLayout />}>
          <Route path="/" element={<Projects />} />
          <Route path="/projects" element={<Projects />} />
          <Route path="/projects/:id" element={<ProjectDetail />} />
          <Route path="/inventory" element={<Inventory />} />
          <Route path="/inventory/:id" element={<ResourceDetail />} />
          <Route path="/accounts" element={<Accounts />} />
          <Route path="*" element={<Navigate to="/projects" replace />} />
        </Route>
      </Routes>
    </BrowserRouter>
  )
}

function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <Root />
    </QueryClientProvider>
  )
}

export default App
