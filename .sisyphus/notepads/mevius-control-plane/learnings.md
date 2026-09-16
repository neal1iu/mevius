# Learnings — mevius-control-plane

> Conventions, patterns, discovered gotchas. **APPEND ONLY — never overwrite.**

## 环境基线 (2026-09-16 实测)
- go 1.27.1 linux/amd64
- node v24.14.1 / npm 11.11.0
- docker 29.4.0 / docker compose v5.1.3
- jq 1.7
- openssl 3.0.13
- `sqlite3` CLI 与 `golangci-lint` 初始缺失 → Atlas 已安装（见 problems.md 记录）
- 仓库 greenfield：初始只有 README.md/LICENSE/.gitignore

## 计划锚定产物（实现前必读，位于 plan 文件）
- 领域模型 DDL（plan L57-75）— 4 表 + UNIQUE(slot_id,account_id,external_id) + INDEX(account_id,external_id)
- Slot config typed schemas（plan L80-84）
- 能力矩阵（plan L86-94）— 矩阵外一律 501
- Provider 操作配方（plan L96-117）
- Provider 插件接口（plan L119-129）
- 刷新引擎规范（plan L131-132）
- 目录布局（plan L134-150）
- 依赖白名单（plan L152-154）

## 关键约定
- 所有时间戳 UTC RFC3339
- token 永不出现在 API 响应/日志/错误体；上游错误体需 scrub
- SDK 类型严禁泄漏出 `internal/provider/{provider}/` 包
- 查询/UI 永远走 account_id，不假设"唯一账户"
- 测试禁止调用真实 provider API（httptest fixture + committed JSON）

## [2026-09-16T15:06Z] Task: T1 — Go backend scaffold
- chi v5.3.2 added; module `mevius` (go 1.27.1). golangci-lint v2.13.2 config MUST start with `version: "2"`; used `linters: {default: standard, enable: [bodyclose, errorlint, misspell, unconvert]}` + `formatters: [gofmt]` → 0 issues.
- **chi gotcha (important for all later API tasks)**: chi SKIPS middleware for routes that don't match — a request to an unregistered path hits NotFound directly, bypassing `Use()` middleware. Fix: register a catch-all `r.Handle("/*", http.NotFoundHandler())` inside the protected group so bearer auth runs for every unmatched `/api/v1/*` path. Exact routes (e.g. `/health`) still take precedence over `/*`.
- `http.Error` body is 13 bytes ("unauthorized\n"), chi default 404 body is 19 bytes — used in QA assertions.
- Config fail-fast: `hex.DecodeString` + length==64 check for master key; errors name the env var. `os.Exit(1)` in main via `run() error` + stderr print.
- Request logger: custom `statusRecorder` wrapper (WriteHeader/Write) to capture status+bytes; logs only method/path/status/bytes/duration — never Authorization.
- Makefile `build` outputs `./mevius` (gitignored via `/mevius`); `lint` uses `$(go env GOPATH)/bin/golangci-lint`.

## 2026-09-16T23:18Z Task: T2 (前端 scaffold)
- 工具链实测: Vite 8.3.0 / React 19.2.8 / TypeScript 6.0.2 / Tailwind v4 (@tailwindcss/vite) / shadcn CLI 4.21.0 (preset `radix-nova`, icon lucide) / @tanstack/react-query 5.103.1 / react-router-dom 7.18.4
- **坑 — TS 6.0 deprecates `baseUrl`**: shadcn 官方 Vite 指南要求加 `baseUrl`，但 TS 6.0 报 TS5101。解决：去掉 `baseUrl`，只保留 `paths: { "@/*": ["./src/*"] }`（TS 4.1+ 允许 paths 相对 tsconfig 解析）。
- **坑 — shadcn init 非交互**: `npx shadcn@latest init -t vite -b radix -p nova -y`。注意 `-d/--defaults` = `--template=next --preset=base-nova`（是 next 模板，Vite 项目不可用）；`radix` 对应旧 "new-york"（白名单明确 radix）。init 会自动写入 index.css 并新增 `@fontsource-variable/geist`、`radix-ui`(umbrella)、`lucide-react`、`cn`、`class-variance-authority`、`tw-animate-css`、`shadcn`(CLI 包)。
- **坑 — shadcn v4 nova 组件 import `cn` 来自 `"cn"` 包直接**（非 `@/lib/utils`）；`src/lib/utils.ts` 内容为 `export { cn } from "cn"`。自写代码从 `@/lib/utils` 导入亦可。
- **坑 — @tanstack/react-query 首次 install 报 query-core@5.103.1 not found**（registry 传播/缓存），重试即成功。
- **坑 — vite.config 用 `import.meta.dirname`**（Node 24 ESM，`"type":"module"`），不能用 `__dirname`（会 TS 报错）。
- **坑 — Playwright MCP 本环境锁定 `channel=chrome` + 开启 sandbox**，root 下启动报 "Running as root without --no-sandbox"。`npx playwright install chrome --with-deps` 装 Google Chrome 153 后仍被 sandbox 拦。**可行替代**：独立 playwright 脚本 + `chromium.launch({ channel:'chrome', args:['--no-sandbox'] })`（已装 playwright@1.63.0 于 /tmp/opencode）。
- **API 客户端约定**（后续 T19-T23 复用）: `src/lib/api.ts` 导出 `apiFetch<T>(path, init?)`，base 常量 `'/api'`，自动注入 `Authorization: Bearer <localStorage.mevius_token>`；`ApiError` 携带 `status`；401 → `clearToken()` + `setUnauthorizedHandler` 回调（App 里回调清 query cache 并回 Gate）；204 → undefined。DEV 模式暴露 `window.meviusApi` 供 QA 直接调 `apiFetch`。
- **布局壳约定**: `App.tsx` 挂 `QueryClientProvider` + `BrowserRouter`；无 token 渲染 `<Gate/>`（不包 router）；路由 `/`、`/projects`、`/projects/:id`、`/accounts` 全占位页，侧边栏 `Sidebar.tsx` 含 "Projects"/"Accounts" 两个 NavLink（英文标签，Playwright 依赖）。
- **端口约定**: `server.port=3000`（固定，QA 依赖）；proxy `/api` → `http://localhost:8080`。
- QA 通过：gate 流程（password 可见→填 "abc"→点 "Save"→localStorage=abc→nav 含 Projects/Accounts）；auth header 拦截 `Bearer abc`（`/api/v1/health` 返回 502 因 T1 后端未起，但 header 捕获正确）。

## [2026-09-17T00:00Z] Task: T4 — Crypto envelope (secretbox)
- `golang.org/x/crypto v0.57.0` added (nacl/secretbox). Only new dep, matches whitelist.
- Envelope format: `v1:{keyID}:{base64(nonce)}:{base64(ct)}`. keyID = first 8 hex chars of SHA256(key).
- `secretbox.Seal` does NOT prepend nonce — nonce stored separately in envelope.
- `secretbox.Open` returns `([]byte, bool)` — bool=false means auth failure (wrong key/tampered).
- `ParseMasterKey` validates length==64 + hex decode. Used by config.Load later.
- Test count: 7 (5 required + 2 bonus: ParseMasterKey errors table + malformed envelope table).
- `go vet ./internal/crypto/` clean.
- Evidence: task-4-crypto-tests.txt, task-4-bad-key.txt.

## [2026-09-17T00:00Z] Task: T7 — Projects+Slots service & API
- `modernc.org/sqlite` (not `mattn/go-sqlite3`) — for UNIQUE constraint checking, pre-check via GetByName instead of parsing driver-specific errors
- Dynamic IN queries: modernc/sqlite does not support array parameters; manually build positional `?,?,?` placeholders in code
- `json.RawMessage` can be `nil` — normalize to `"{}"` before insert
- Slot type is immutable: UpdateSlot preserves existing type (only name + config changed)
- Project detail aggregation: GetProject (1 query) → ListSlotsByProject (1 query) → collect slot IDs → ListBindingsBySlots (1 query with IN) — 3 queries total, no N+1
- Migration 0002 added `project.updated_at` column to support PATCH endpoint
- Added hand-written queries to sqlc-generated store: GetProjectByName, UpdateProject, UpdateSlot (name+config), ListBindingsBySlots

## [2026-09-17T02:10Z] Task: T6 — Accounts service & API
- Service layer `internal/service/account.go` accepts `*store.Queries`, `[32]byte` key, and `*provider.Registry` for the three-dependency pattern
- `AddAccount` flow: validate token via registry → encrypt via crypto.Encrypt → store via sqlc → zero out token before return
- Token is never returned: `TokenEncrypted` field is zeroed in `toDomainAccount()` helper and `GetAccount`/`ListAccounts`
- `DeleteAccount` relies on FK CASCADE (binding → provider_account ON DELETE CASCADE) — no provider API calls, just DB delete
- Stub provider in `internal/provider/stub.go`: returns unauthorized for empty/"bad"-prefixed tokens, fixed meta for valid tokens
- API uses existing `writeJSON(w, v, status)` / `writeError(w, msg, status)` helpers from projects.go — important to match argument order
- Wire in `cmd/mevius/main.go`: `store.Open()` → `crypto.ParseMasterKey()` → `provider.NewRegistry()` + register stubs → `service.NewAccountService()` → pass to `api.NewRouter()`

## [2026-09-17T03:15Z] Task: T8 — Pull-only refresh engine
- `internal/service/refresh.go`: `RefreshEngine` with in-memory TTL cache (`map[string]time.Time` + `sync.RWMutex`)
- `GetBindingStatus`: checks TTL → hit → load from DB + return; miss → `singleflight.Do(bindingID)` → `Inspector.GetResource` → write back `cached_meta+sync_status+last_synced_at`
- Error mapping: 404→orphaned, 401→auth_error, other→error+30s negative TTL
- `Refresh(bindingID)`: deletes TTL entry → calls GetBindingStatus (bypasses TTL, still uses singleflight)
- `Invalidate(keys...)`: deletes TTL entries — used after create/delete/deploy/DNS mutations
- `FanOutRefresh(accountID, externalID)`: one `GetResource` call for all bindings sharing same (account_id,external_id)
- Test strategy: `fakeQuerier` implements `store.Querier` with in-memory maps; `providerAdapter` with function fields for count tracking; 10 test cases all pass under -race
- singleflight dep (`golang.org/x/sync`) was already in go.mod (indirect from pressly/goose)

## [2026-09-17T05:05Z] Task: T16 — Vercel provider core (thin REST client)
- Vercel uses provider.Client.DoReq directly (no SDK, unlike GitHub's go-github). No import of gogithub pattern — Vercel provider is pure REST.
- team_id stored in Account.Meta.Raw["team_id"]; passed through via ?teamId= query param on every request when present. Preserved in ValidateCredentials output.
- Vercel API quirks: /v9/projects returns `{"projects":[...]}` (plural wrapper), framework can be null in JSON, /v10/projects for creation (different version from list), DELETE returns 204 No Content.
- For null framework values in project JSON, used `*string` pointer type to distinguish null from empty string — only include "framework" in meta when non-nil.
- Authorization header format: `Bearer {token}` (same as Vercel CLI convention).
- Fixture pattern: same readFixture helper pattern as GitHub provider in github_test.go.
- All 15 tests pass with zero real API calls (all httptest). Added TestTeamIDPropagation_OnProjectsList to verify teamId propagates to ListExternalResources too.
- Main.go updated: `reg.Register(vcprov.NewProvider(provider.ProviderBaseURL("vercel")))` replaces stub.

## [2026-09-17T06:00Z] Task: T11 — Binding lifecycle service & API
- `internal/service/binding.go`: `capabilityMatrix` maps ResourceKind→[]ProviderType; `ValidateCapability` returns `ErrUnsupportedCombo` for invalid slot×provider combos (maps to 501)
- `Bind` flow: validate matrix → insert binding (UNIQUE constraint → 409 via `isUniqueConstraintError`) → Inspector.GetResource for initial cached_meta → update sync_status=ok + meta
- `Discover` is purely transient: calls provider.ListExternalResources, no persistence
- `Unbind` is just a DB delete — no provider API calls
- `Refresh` delegates to T8 RefreshEngine.Refresh (bypasses TTL)
- `ListBySlot` passes through `sync_status`/`cached_meta` from DB (updated by T8 engine)
- API routes registered in `registerBindingRoutes` inside router.go protected group: `GET /accounts/{id}/discover?kind=`, `POST /slots/{id}/bindings`, `DELETE /bindings/{id}`, `POST /bindings/{id}/refresh`, `GET /slots/{id}/bindings`
- For UNIQUE constraint detection with modernc/sqlite: use `strings.Contains(err.Error(), "UNIQUE constraint")` — portable and avoids driver-specific imports

## [2026-09-17T08:00Z] Task: T13 — CF provider core (credential validation, account resolution, workers CRUD)
- CloudflareProvider uses `provider.NewClient` (raw HTTP, no SDK) — same as Vercel pattern, since worker upload needs custom multipart encoding
- CF API responses wrapped in `{"success":bool, "result":..., "errors":[...], "messages":[...]}` — custom `cfResponse` struct for parsing
- `ValidateCredentials` does two calls: GET /user/tokens/verify (status must be "active") → GET /accounts (first account used as meta.account_id + meta.account_name in Raw)
- Workers list: GET /accounts/{aid}/workers/scripts returns `[{"id","created_on","modified_on"}]`
- Worker create: PUT /accounts/{aid}/workers/scripts/{name} requires multipart/form-data with metadata part (`{"main_module":"worker.js"}`) and script part (`application/javascript+module`) — uses `mime/multipart` + `textproto.MIMEHeader`
- Placeholder script content: `export default { async fetch(request) { return new Response("mevius placeholder", { status: 200 }); } }`
- Error mapping: 401→KindUnauthorized, 404→KindNotFound (via provider.MapHTTP from httpclient.go DoReq)
- Test pattern matches GitHub provider: httptest + readFixture helper + testdata JSON files
- After DoReq returns error for >=400 status, response body is NOT returned for further parsing (DoReq returns the body + error, but the error is returned instead of body). Solution: provider.DoReq returns body bytes even on error (the error is returned separately), so we don't need additional error parsing for CF error bodies — MapHTTP handles it via status code
- Registered in main.go: `cfprov.NewProvider(provider.ProviderBaseURL("cloudflare"))` replaces stub
- go-github v66 method names: `CreateWorkflowDispatchEventByFileName`, `CreateWorkflowDispatchEventRequest`, `ListRepositoryWorkflowRuns`, `ListWorkflowRunsOptions`
- `GetWorkflowRunLogs` returns `(*url.URL, *Response, error)` internally following 0 redirects. For 410 it returns `(nil, nil, err)` so can't inspect response status. Better: `client.NewRequest` + `http.Client{CheckRedirect: http.ErrUseLastResponse}` to handle redirects manually and detect 410.
- Zip handling: download to memory (2MB cap via io.LimitReader), extract via archive/zip, sort filenames, concatenate with newlines, tail 256KB, set truncated flag
- Synthetic zip fixtures created inline via `archive/zip` writer — no committed binary fixtures needed
- Httptest closures can't reference `ts` before declaration; use `r.Host` from request to construct redirect Location URL

## Vercel Deploy (T17 — 2026-09-17)
- Redeploy spike conclusion: POST /v13/deployments with `deploymentId` from latest deployment is simpler than reconstructing gitSource. The `deploymentId` inherits all project settings and env vars automatically.
- TriggerDeploy flow: GET /v6/deployments?projectId=&limit=1 → extract latest UID → POST /v13/deployments with `{"deploymentId":"...","name":"...","target":"production"}`
- ListDeployments: GET /v6/deployments?projectId=&limit=20 → map `readyState` (BUILDING/INITIALIZING→in_progress, READY→completed/success, ERROR/CANCELED/BLOCKED→completed/failure, QUEUED→queued)
- GetBuildLogs: GET /v3/deployments/{id}/events → extract `payload.text` from each event array element → concatenate → tail 256KB → set truncated flag
- Events endpoint returns array of objects with `type` (command/stdout/exit/etc), `created` (unix ms), `payload` object containing `text` (log line string)
- DeployEvent domain type has no URL field — only ID, Status, CreatedAt, UpdatedAt
- Test pattern: httptest with request method/path assertions, readFixture helper shared across all test files
- Pre-existing issue: GetResource in vercel.go tries /v5/domains/{id} first, then falls back to /v9/projects/{id}. TestGetResource_Success needed update to handle this two-call pattern.

## T14 — CF Pages (2026-09-17)

### What was built
- `internal/provider/cloudflare/pages.go`: Pages CRUD (list/inspect/create/delete), Deployer (TriggerDeploy, ListDeployments), LogFetcher (GetBuildLogs with fallback)
- `cloudflare.go` updated: `ListExternalResources` handles `static-site` kind; `CreateResource` dispatches on `spec.Kind`; `DeleteResource` falls back from workers→pages on 404

### Spike conclusion
CF Pages REST API does not expose raw build log output via any public endpoint. The deployment detail endpoint provides stage-level status/timing only. GetBuildLogs returns a stage summary (text) with `meta.fallback="dashboard_link"` and `meta.url` pointing to the Cloudflare Dashboard.

### Trigger matrix
- Git-connected project → inspect → retry latest deployment → DeployEvent
- Direct Upload project → inspect → unsupported error
- In-progress deployment → inspect → upstream error ("already in progress")

### Test pattern
- Same httptest + fixtures pattern as T13 (workers)
- 8 testdata fixtures for projects, deployments, retry responses
- Contract tests cover all CRUD operations, trigger matrix (3 states), deployment listing, and log fallback

## T18 — Vercel DNS (2026-09-17)

### What was built
- `internal/provider/vercel/dns.go`: DNSManager implementation for Vercel-managed domains
- `internal/api/dns.go`: DNS API routes at `/bindings/{id}/dns-records` with provider dispatch
- `vercel.go` GetResource extended: tries `/v5/domains/{domain}` first for dns-domain (returns verified status), falls back to `/v9/projects/{id}` for static-site
- Updated `router.go` and `main.go` to wire DNS routes (passes store.Querier and provider.Registry to NewRouter)

### Vercel DNS API endpoints
- List records: GET /v5/domains/{domain}/records → `{"records": [{id,slug,name,type,value,creator,domain,ttl,createdAt,updatedAt}]}`
- Create record: POST /v2/domains/{domain}/records with `{"type","name","value","ttl"}` → `{"uid":"rec_...","updated":timestamp}`
- Update record: PATCH /v1/domains/records/{recordId} with any subset of `{"type","name","value","ttl"}` → full record object with id,slug,name,type,value,domain,ttl,creator,createdAt
- Delete record: DELETE /v2/domains/{domain}/records/{recordId} → 200 OK
- Domain detail (inspector): GET /v5/domains/{domain} → `{"domain":{"verified":bool,...}}`

### DNS API route design
- Routes at `/bindings/{id}/dns-records` (GET list, POST create, PATCH/{recordID} update, DELETE/{recordID} delete)
- `resolveDNSManager` helper: gets binding→account→provider→asserts DNSManager→returns (dnsMan, provAcct, zoneID, status, msg)
- Error handling: provider.Errors map to 400, internal errors to 500, not-found to 404
- Router signature updated: `NewRouter` now accepts `q store.Querier, reg *provider.Registry` for DNS route wiring

### GetResource dual-behavior
- Since GetResource doesn't receive the resource kind, it tries `/v5/domains/{id}` first
- If domain endpoint succeeds with a valid domain response → returns verified status in cached_meta
- If domain endpoint fails → falls back to `/v9/projects/{id}` for static-site resources
- This allows the refresh engine to work for both dns-domain and static-site bindings via the same method

### Test pattern
- 8 DNS-specific tests: list, list 404, create, create zero-TTL, update, delete, delete 404, rate limit
- 3 GetResource domain tests: verified, unverified, not-found (with two-call fallback assertion)
- All use httptest with testdata JSON fixtures
- `go test ./internal/provider/vercel/ -count=1` passes (all previous tests + 11 new tests)

## T12 — Deploy trigger, history, streaming logs (2026-09-17)

### What was built
- `internal/service/deploy.go`: `DeployService` with `TriggerDeploy`, `ListDeployments` (30s TTL cache), `GetLogs`
- `internal/api/deploys.go`: three routes under `/bindings/{id}/deploys` — POST trigger (202), GET list (array with in_progress), GET logs (text/plain streaming with X-Truncated)
- Wired into `router.go` protected group and `main.go`

### Key design decisions
- `TriggerDeploy` fetches binding+slot+account → asserts Deployer capability → calls provider → invalidates refresh cache
- `ListDeployments` uses in-memory TTL cache (30s) to avoid repeated provider calls for the same binding
- `GetLogs` streaming response: `Content-Type: text/plain`, `X-Truncated: true` header when truncated, tail param caps lines
- Error scrubbing: `scrubProviderErr` wraps provider.Errors with `provider.ScrubTokens` on ProviderMsg; non-provider errors also scrubbed
- `in_progress` computed in API layer: status in {queued,in_progress} → true
- `provider.ScrubTokens` exported from `internal/provider/errors.go` (was unexported `scrubTokens`, needed for service layer reuse)
- No deployment persistence — all calls go directly to provider Deployer/LogFetcher

### Route pattern
- Register: `registerDeployRoutes(r, deploySvc)` called after DNS routes in router.go
- Router accepts `*service.DeployService` and `*service.RefreshEngine` params

## [2026-09-17T06:00Z] Task: T21 — Bindings UI
- `web/src/pages/SlotDetail.tsx` created: binding cards in responsive grid (sm:grid-cols-2), status badges with distinct colors per sync_status, manual refresh per-card with loading state, unbind with confirmation dialog
- `BindDialog` component: fetches accounts → filters by capability matrix (CAPABILITY_MATRIX constant matching backend) → account dropdown (radio-like styled buttons) → discover list (searchable radio list) → submit. Uses `useMutation` for discover/bind calls
- Status badge colors: ok=green, error=red, auth_error=orange, orphaned=gray "remote deleted", never=blue
- Unbind confirmation message: "This only unbinds the binding — it does not delete the remote resource."
- Binding cards show: provider icon (2-letter initials), display_name, external_id, sync_status badge, relative last_synced_at, Refresh+Unbind buttons
- In-flight refresh: button disabled while refreshing via Set<string> of refreshing IDs
- Toast notifications: simple fixed position toast with auto-dismiss (3s), green for success, red for error
- Provider initials extracted from PROVIDER_LABELS map (first 2 uppercase chars)
- Route added at `/projects/:projectId/slots/:slotId` in App.tsx
- API functions added to `api.ts`: getSlot, listAccounts, listBindings, discoverResources, bindResource, refreshBinding, unbindBinding with proper types
- Pre-existing lint: Projects.tsx had unused `cn` import — fixed during this task to unblock build

## [2026-09-17T09:00Z] Task: T19 — Accounts UI
- `web/src/pages/Accounts.tsx`: complete accounts management page with table, add dialog, delete dialog
- Table columns: provider icon (2-letter initials in colored box), provider name, label, identity/scopes meta summary, created_at date, delete action button
- MetaSummary component: GitHub shows login + scopes + missing_scopes warning badge (pale-yellow); Cloudflare shows account_name; Vercel shows username + optional team_id
- ProviderIcon uses pale-green (github), pale-red (cloudflare), pale-blue (vercel) background colors
- `useAccounts`/`useAddAccount`/`useDeleteAccount` with TanStack Query; add mutation refetches list on success
- AddAccountDialog: Select for provider (radix-ui/select), Input for label + token (type=password), inline error display in pale-red box
- DeleteAccountDialog: "This will unbind all associated bindings. Remote resources will not be affected." — matches binding UI unbind message style
- Error handling: `parseProviderError` tries JSON parse of ApiError.message for `{kind, message}` payload; falls back to plain error message; displayed inline in dialog
- Skeleton loading: 3 animated placeholder rows during isLoading
- Empty state: centered message + "Add your first account" outline button
- No token in DOM: password input is type=password, never rendered as text; response has no token field
## [2026-09-17T10:00Z] Task: T20 — Projects+Slots UI (typed config forms)
- `web/src/pages/Projects.tsx`: card grid (name/description/created_at) + new project dialog (name+description) + delete confirmation dialog
- `web/src/pages/ProjectDetail.tsx`: project header (back link, name, description, created_at, slot count) + slot list (type icon in border box, type label uppercase, name, binding count, status dot) + add slot dialog with dynamic form per type
- 4 slot type forms: repo (name, private toggle, description, workflow_id, workflow_ref), compute (name, compatibility_date), static-site (name, framework, build_command, output_dir, production_branch), dns-domain (info text only — no config fields)
- Private toggle uses custom switch button (radix-ui/Switch alternative) with aria-checked
- Status dot component: ok=emerald, error=red, auth_error=amber, orphaned/never=muted gray — worst status computed per slot across all its bindings
- Form error mapping: checks err.message.toLowerCase() for "name" / "name is required" → sets `formErrors.name` → shown below input as destructive text + AlertCircle icon
- Dialog uses radix-ui/Dialog with Portal/Overlay/Content pattern matching T19 Accounts UI
- Slot type dropdown uses native `<select>` styled with tailwind (border-input, rounded-lg)
- Empty states: centered messages with icon, text, subtext — matching Projects page style
- Slot row links to `/slots/${id}` (placeholder for T21/T22/T23)
- lucide-react icons: Code (repo), Cpu (compute), Globe (static-site), Globe2 (dns-domain), Plus, Trash2, ArrowLeft, FolderKanban, AlertCircle
- `cn` from `@/lib/utils` reused for conditional class merging

## [2026-09-17T10:30Z] Task: T28 — Frontend tests-after (vitest)
- vitest v5.0.1 installed with @testing-library/react, @testing-library/jest-dom, @testing-library/user-event, jsdom
- vitest.config.ts: `environment: "jsdom"`, `globals: true`, `setupFiles: ["src/test-setup.ts"]`, alias `@` → `./src`
- Native `<select>` (used in ProjectDetail slot type form): use `fireEvent.change(select, { target: { value } })` — `userEvent.click` on option text doesn't work for native selects
- Radix UI Select in jsdom: `PointerEvent.prototype.hasPointerCapture` missing — polyfill in test-setup.ts: `Element.prototype.hasPointerCapture = () => false`
- Radix Select has TWO elements with `role="combobox"` — the visible trigger button + a hidden native `<select>`. Use `getAllByRole("combobox")[0]` for the trigger
- Radix Portal options are found in the DOM tree (not in a shadow DOM), so `findByText` works after clicking the trigger
- MemoryRouter with Routes/Route needed for `useParams` to work: `<MemoryRouter initialEntries={["/projects/proj-1"]}><Routes><Route path="/projects/:id" element={<ProjectDetail />} /></Routes></MemoryRouter>`
- Mutation success triggers `invalidateQueries` which re-fetches — for `mockResolvedValueOnce`, order matters: first the mutation response, then the invalidation re-fetch
- 25 tests across 3 files (api.test.ts, slot-config-form.test.tsx, Accounts.test.tsx) all pass
- Build unchanged: `npm run build` still succeeds
