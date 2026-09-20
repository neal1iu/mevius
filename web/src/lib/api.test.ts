import { describe, it, expect, vi, beforeEach } from "vitest"
import { apiFetch, ApiError, setUnauthorizedHandler, getToken, setToken } from "./api"

beforeEach(() => {
  vi.restoreAllMocks()
  localStorage.clear()
})

describe("apiFetch", () => {
  it("prefixes path with /api and sends Authorization header when token exists", async () => {
    setToken("test-token")
    const mock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ ok: true }), {
        status: 200,
        headers: { "content-type": "application/json" },
      }),
    )

    await apiFetch("/projects")

    const [, init] = mock.mock.calls[0]
    expect(init?.headers).toBeInstanceOf(Headers)
    const headers = init!.headers as Headers
    expect((mock.mock.calls[0] as [string])[0]).toBe("/api/v1/projects")
    expect(headers.get("Authorization")).toBe("Bearer test-token")
    expect(headers.get("Accept")).toBe("application/json")
  })

  it("does not send Authorization when no token is stored", async () => {
    const mock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ ok: true }), {
        status: 200,
        headers: { "content-type": "application/json" },
      }),
    )

    await apiFetch("/projects")

    const headers = (mock.mock.calls[0][1] as RequestInit).headers as Headers
    expect(headers.get("Authorization")).toBeNull()
  })

  it("sets Content-Type to application/json when body is present", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ id: "1" }), {
        status: 200,
        headers: { "content-type": "application/json" },
      }),
    )

    await apiFetch("/projects", { method: "POST", body: JSON.stringify({ name: "test" }) })

    const headers = vi.mocked(fetch).mock.calls[0][1]!.headers as Headers
    expect(headers.get("Content-Type")).toBe("application/json")
  })

  it("throws ApiError with parsed error body", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ error: "not found" }), {
        status: 404,
        headers: { "content-type": "application/json" },
      }),
    )

    const err = await apiFetch("/missing").catch((e) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect(err.status).toBe(404)
    expect(err.message).toBe("not found")
  })

  it("falls back to message field when error key is absent", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ message: "bad request" }), {
        status: 400,
        headers: { "content-type": "application/json" },
      }),
    )

    const err = await apiFetch("/bad").catch((e) => e)
    expect(err.message).toBe("bad request")
  })

  it("falls back to statusText when body is not json", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response("not json", { status: 500, statusText: "Internal Server Error" }),
    )

    const err = await apiFetch("/fail").catch((e) => e)
    expect(err.message).toBe("Internal Server Error")
  })

  describe("401 → gate callback", () => {
    it("clears token, fires unauthorized handler exactly once, and throws", async () => {
      setToken("expired")
      const handler = vi.fn()
      setUnauthorizedHandler(handler)

      vi.spyOn(globalThis, "fetch").mockResolvedValue(
        new Response(null, { status: 401 }),
      )

      const err = await apiFetch("/secret").catch((e) => e)
      expect(err).toBeInstanceOf(ApiError)
      expect(err.status).toBe(401)
      expect(getToken()).toBeNull()
      expect(handler).toHaveBeenCalledTimes(1)
    })

    it("still clears token when no handler is set", async () => {
      setToken("expired")
      setUnauthorizedHandler(null)

      vi.spyOn(globalThis, "fetch").mockResolvedValue(
        new Response(null, { status: 401 }),
      )

      await apiFetch("/secret").catch(() => {})
      expect(getToken()).toBeNull()
    })
  })

  it("returns undefined for 204 No Content", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(null, { status: 204 }))

    const result = await apiFetch<void>("/delete", { method: "DELETE" })
    expect(result).toBeUndefined()
  })

  it("returns text when response is not JSON", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response("plain text", {
        status: 200,
        headers: { "content-type": "text/plain" },
      }),
    )

    const result = await apiFetch<string>("/text")
    expect(result).toBe("plain text")
  })
})
