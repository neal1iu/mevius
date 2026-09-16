import { describe, it, expect, vi, beforeEach } from "vitest"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import Accounts from "@/pages/Accounts"

const mockAccounts = [
  {
    id: "acct-1",
    provider: "github",
    label: "my-github",
    meta: {
      login: "octocat",
      scopes: ["repo", "user"],
      missing_scopes: ["workflow", "admin:org"],
    },
    created_at: "2025-01-01T00:00:00Z",
  },
  {
    id: "acct-2",
    provider: "github",
    label: "limited",
    meta: {
      login: "minimal",
      scopes: ["repo"],
      missing_scopes: [],
    },
    created_at: "2025-01-01T00:00:00Z",
  },
  {
    id: "acct-3",
    provider: "cloudflare",
    label: "my-cf",
    meta: { account_name: "Cloudflare User", account_id: "cf-123" },
    created_at: "2025-01-01T00:00:00Z",
  },
  {
    id: "acct-4",
    provider: "vercel",
    label: "my-vercel",
    meta: { username: "vercel-user", team_id: "team_abc" },
    created_at: "2025-01-01T00:00:00Z",
  },
]

function renderAccounts() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <Accounts />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  vi.restoreAllMocks()
  localStorage.clear()
})

describe("Accounts page", () => {
  it("renders missing_scopes badge when present", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify(mockAccounts), {
        status: 200,
        headers: { "content-type": "application/json" },
      }),
    )

    renderAccounts()

    const badge = await screen.findByText(/missing:/i)
    expect(badge).toBeTruthy()
    expect(badge.textContent).toContain("workflow")
    expect(badge.textContent).toContain("admin:org")
  })

  it("does not show missing_scopes badge when array is empty", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify(mockAccounts), {
        status: 200,
        headers: { "content-type": "application/json" },
      }),
    )

    renderAccounts()

    const badges = await screen.findAllByText(/missing:/i)
    expect(badges).toHaveLength(1)
  })

  it("renders login name for GitHub accounts", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify(mockAccounts), {
        status: 200,
        headers: { "content-type": "application/json" },
      }),
    )

    renderAccounts()

    await screen.findByText("octocat")
    expect(screen.getByText("octocat")).toBeTruthy()
  })

  it("renders account name for Cloudflare accounts", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify(mockAccounts), {
        status: 200,
        headers: { "content-type": "application/json" },
      }),
    )

    renderAccounts()

    await screen.findByText("Cloudflare User")
    expect(screen.getByText("Cloudflare User")).toBeTruthy()
  })

  it("renders username and team ID for Vercel accounts", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify(mockAccounts), {
        status: 200,
        headers: { "content-type": "application/json" },
      }),
    )

    renderAccounts()

    await screen.findByText("vercel-user")
    expect(screen.getByText("vercel-user")).toBeTruthy()
    expect(screen.getByText(/team:/)).toBeTruthy()
  })

  it("shows error state when add account API fails with missing_scopes-like error", async () => {
    vi.spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(
        new Response(JSON.stringify(mockAccounts), {
          status: 200,
          headers: { "content-type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ error: "missing_scopes: workflow" }), {
          status: 400,
          headers: { "content-type": "application/json" },
        }),
      )

    const user = userEvent.setup()
    renderAccounts()

    const addButton = await screen.findByRole("button", { name: /add account/i })
    await user.click(addButton)

    const providerTrigger = screen.getAllByRole("combobox")[0]
    await user.click(providerTrigger)

    const githubOptions = await screen.findAllByText("GitHub")
    const githubOption = githubOptions.find(
      (el) => el.closest('[role="option"]') || el.getAttribute("role") === "option",
    ) ?? githubOptions[githubOptions.length - 1]
    await user.click(githubOption)

    const tokenInput = screen.getByPlaceholderText(/paste your api token/i)
    await user.type(tokenInput, "ghp_test")

    const submitButton = screen.getByRole("button", { name: /add account$/i })
    await user.click(submitButton)

    const errorMsg = await screen.findByText(/missing_scopes/i)
    expect(errorMsg).toBeTruthy()
  })

  it("handles empty accounts state", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify([]), {
        status: 200,
        headers: { "content-type": "application/json" },
      }),
    )

    renderAccounts()

    const emptyMsg = await screen.findByText(/no accounts configured/i)
    expect(emptyMsg).toBeTruthy()
  })
})