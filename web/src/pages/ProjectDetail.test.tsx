import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ProjectDetail from './ProjectDetail'

function json(value: unknown, status = 200) {
  return new Response(JSON.stringify(value), { status, headers: { 'content-type': 'application/json' } })
}

function renderDetail() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(<QueryClientProvider client={client}><MemoryRouter initialEntries={['/projects/project-1']}><Routes><Route path="/projects/:id" element={<ProjectDetail />} /></Routes></MemoryRouter></QueryClientProvider>)
}

const repo = { id: 'repo-1', connection_id: 'conn-1', provider_product_id: 'github.repositories', resource_kind: 'git_repo', external_id: 'acme/repo', display_name: 'repo', lifecycle_mode: 'imported', sync_status: 'ok', created_at: 'now', updated_at: 'now' }

beforeEach(() => {
  vi.restoreAllMocks()
})

describe('Project resource roles', () => {
  it('offers only product-compatible roles and submits the selected role', async () => {
    let submitted: Record<string, unknown> | undefined
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
      const path = String(input)
      if (path.endsWith('/projects/project-1/resources') && init?.method === 'POST') {
        submitted = JSON.parse(String(init.body))
        return json({ id: 'link-1', project_id: 'project-1', resource_instance_id: 'repo-1', ...submitted, created_at: 'now', updated_at: 'now' }, 201)
      }
      if (path.endsWith('/projects/project-1')) return json({ project: { id: 'project-1', name: 'Demo', created_at: 'now', updated_at: 'now' }, resources: [], relations: [] })
      if (path.endsWith('/resource-instances')) return json([repo])
      if (path.endsWith('/catalog/providers')) return json({ resource_roles: [{ id: 'source', display_name: 'Source', description: 'Source code' }, { id: 'frontend', display_name: 'Frontend', description: 'Web UI' }], providers: [{ id: 'github', display_name: 'GitHub', products: [{ id: 'github.repositories', provider_id: 'github', display_name: 'Git Repository', resource_kind: 'git_repo', compatible_roles: ['source'], capabilities: [] }] }] })
      throw new Error(`unexpected request: ${path}`)
    })
    const user = userEvent.setup()
    renderDetail()

    await user.click(await screen.findByRole('button', { name: 'Attach resource' }))
    await user.selectOptions(screen.getByLabelText('Resource'), 'repo-1')
    expect(screen.getByRole('option', { name: 'Source' })).toBeInTheDocument()
    expect(screen.queryByRole('option', { name: 'Frontend' })).not.toBeInTheDocument()
    await user.selectOptions(screen.getByLabelText('Role'), 'source')
    const attach = screen.getByRole('button', { name: 'Attach' })
    expect(attach).toBeEnabled()
    await user.click(attach)

    await waitFor(() => expect(submitted).toMatchObject({ resource_instance_id: 'repo-1', role: 'source' }))
  })
})
