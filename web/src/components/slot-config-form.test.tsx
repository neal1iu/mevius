import { describe, it, expect, vi, beforeEach } from "vitest"
import { render, screen, fireEvent } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { MemoryRouter, Route, Routes } from "react-router-dom"
import ProjectDetail from "@/pages/ProjectDetail"

const PROJECT_RESPONSE = {
  project: { id: "proj-1", name: "Test Project", created_at: "2025-01-01T00:00:00Z", updated_at: "2025-01-01T00:00:00Z" },
  slots: [],
  bindings: [],
}

function createMockResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  })
}

function mockFetchProject() {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(createMockResponse(PROJECT_RESPONSE))
}

function renderProjectDetail() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={["/projects/proj-1"]}>
        <Routes>
          <Route path="/projects/:id" element={<ProjectDetail />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

function selectSlotType(value: string) {
  const select = screen.getByRole("combobox") as HTMLSelectElement
  fireEvent.change(select, { target: { value } })
}

beforeEach(() => {
  vi.restoreAllMocks()
  localStorage.clear()
})

describe("SlotConfigForm — type-dependent field rendering", () => {
  beforeEach(() => {
    mockFetchProject()
  })

  async function openAddSlotDialog() {
    const user = userEvent.setup()
    renderProjectDetail()
    const addButton = await screen.findByRole("button", { name: /add slot/i })
    await user.click(addButton)
    return user
  }

  it("renders repo fields: Private toggle, Description, Workflow ID, Workflow Ref", async () => {
    await openAddSlotDialog()
    selectSlotType("repo")
    expect(screen.getByText("Private")).toBeTruthy()
    expect(screen.getByText("Description")).toBeTruthy()
    expect(screen.getByPlaceholderText("e.g. deploy.yml")).toBeTruthy()
    expect(screen.getByPlaceholderText("e.g. main")).toBeTruthy()
  })

  it("renders compute fields: Compatibility Date only", async () => {
    await openAddSlotDialog()
    selectSlotType("compute")
    expect(screen.getByPlaceholderText("e.g. 2025-01-01")).toBeTruthy()
    expect(screen.queryByPlaceholderText("e.g. deploy.yml")).toBeNull()
  })

  it("renders static-site fields: Framework, Build Command, Output Dir, Production Branch", async () => {
    await openAddSlotDialog()
    selectSlotType("static-site")
    expect(screen.getByPlaceholderText("e.g. next")).toBeTruthy()
    expect(screen.getByPlaceholderText("e.g. npm run build")).toBeTruthy()
    expect(screen.getByPlaceholderText("e.g. .next")).toBeTruthy()
    expect(screen.getByPlaceholderText("e.g. main")).toBeTruthy()
  })

  it("renders dns-domain: informational message, no input fields", async () => {
    await openAddSlotDialog()
    selectSlotType("dns-domain")
    expect(screen.getByText(/bind-only/i)).toBeTruthy()
    expect(screen.queryByPlaceholderText("e.g. deploy.yml")).toBeNull()
    expect(screen.queryByPlaceholderText("e.g. 2025-01-01")).toBeNull()
    expect(screen.queryByPlaceholderText("e.g. .next")).toBeNull()
  })
})

describe("SlotConfigForm — required validation", () => {
  it("shows error when name is empty on submit", async () => {
    mockFetchProject()
    const user = userEvent.setup()
    renderProjectDetail()

    const addButton = await screen.findByRole("button", { name: /add slot/i })
    await user.click(addButton)

    const submitButton = screen.getByRole("button", { name: /add slot$/i })
    await user.click(submitButton)

    expect(screen.getByText("Name is required")).toBeTruthy()
  })
})

describe("SlotConfigForm — submit payload shape", () => {
  it("sends correct payload for repo type", async () => {
    mockFetchProject()
    const user = userEvent.setup()
    renderProjectDetail()

    const addButton = await screen.findByRole("button", { name: /add slot/i })
    await user.click(addButton)

    const nameInput = screen.getByPlaceholderText("my-slot")
    await user.type(nameInput, "my-repo")

    const fetchMock = vi.mocked(fetch)
    fetchMock.mockResolvedValueOnce(
      createMockResponse({ id: "slot-1", project_id: "proj-1", name: "my-repo", kind: "repo", provider: "github", created_at: "2025-01-01T00:00:00Z" }),
    )
    fetchMock.mockResolvedValueOnce(
      createMockResponse(PROJECT_RESPONSE),
    )

    const submitButton = screen.getByRole("button", { name: /add slot$/i })
    await user.click(submitButton)

    const allCalls = fetchMock.mock.calls
    const apiCall = allCalls.find(([url]) => url.toString().includes("/projects/proj-1/slots"))
    expect(apiCall).toBeTruthy()
    const body = JSON.parse(apiCall![1]!.body as string)
    expect(body.type).toBe("repo")
    expect(body.name).toBe("my-repo")
    expect(body.config).toEqual({ name: "my-repo", private: false })
  })

  it("sends correct payload for static-site type", async () => {
    mockFetchProject()
    const user = userEvent.setup()
    renderProjectDetail()

    const addButton = await screen.findByRole("button", { name: /add slot/i })
    await user.click(addButton)

    selectSlotType("static-site")

    const nameInput = screen.getByPlaceholderText("my-slot")
    await user.type(nameInput, "my-site")

    const frameworkInput = screen.getByPlaceholderText("e.g. next")
    await user.type(frameworkInput, "next")

    const fetchMock = vi.mocked(fetch)
    fetchMock.mockResolvedValueOnce(
      createMockResponse({ id: "slot-2", project_id: "proj-1", name: "my-site", kind: "static-site", provider: "cloudflare", created_at: "2025-01-01T00:00:00Z" }),
    )
    fetchMock.mockResolvedValueOnce(
      createMockResponse(PROJECT_RESPONSE),
    )

    const submitButton = screen.getByRole("button", { name: /add slot$/i })
    await user.click(submitButton)

    const allCalls = fetchMock.mock.calls
    const apiCall = allCalls.find(([url]) => url.toString().includes("/projects/proj-1/slots"))
    expect(apiCall).toBeTruthy()
    const body = JSON.parse(apiCall![1]!.body as string)
    expect(body.type).toBe("static-site")
    expect(body.name).toBe("my-site")
    expect(body.config.framework).toBe("next")
  })
})

it("does not render dns-domain fields when switching from another type", async () => {
  mockFetchProject()
  const user = userEvent.setup()
  renderProjectDetail()

  const addButton = await screen.findByRole("button", { name: /add slot/i })
  await user.click(addButton)

  selectSlotType("static-site")
  expect(screen.getByPlaceholderText("e.g. .next")).toBeTruthy()

  selectSlotType("dns-domain")
  expect(screen.queryByPlaceholderText("e.g. .next")).toBeNull()
})