# Resource catalog contract (epoch 6)

The canonical model and integration interfaces live in `internal/catalog`.
`internal/integrations` translates the existing GitHub, Cloudflare and Vercel
protocol drivers into these interfaces. `internal/domain` types consumed by
those drivers are protocol DTOs, not the API or database model. New integrations
should implement the catalog interfaces directly, instead of extending the bridge.

## Identity and authorization

A deployment is uniquely identified by `(provider_id, instance_key)`. Endpoint
configuration is versioned and immutable. Create distinct instances for public
cloud, enterprise installations and other deployments. Connections belong to
an instance and represent independent authorizations. Credentials are encrypted;
no connection ID is part of resource identity.

A provider validates credentials, discovers scopes and returns authoritative
scope candidates when binding. The service ignores client-supplied canonical
keys, labels and authorization context after validation. Account and environment
belong in the canonical scope key. A connection can bind multiple scopes;
parent scopes never grant permissions automatically.

Adapters return **complete**, normalized identity parts as strings. The core
encodes them as a compact JSON array. Instance identities contain stable remote
UIDs. Scope identities start with the scope key's parts. Parent identities start
with the parent's identity parts and include a typed, verified parent reference.
Natural identity is explicit. An import matching an existing natural identity
requires `confirm_resource_id`; matching names never establish a rename or
recreation. Natural identities cannot qualify for remote deletion because they
cannot prove the current remote creation. Existing snapshots can be explicitly
refreshed through their own access paths.

`resource_instance` contains shared identity and a public locator. Each
`resource_access` keeps its own observation, capability states, credential
revision and attempt/success timestamps. Reads never combine snapshots or
permissions. Operations require an explicit access when more than one valid
path exists. Refresh can target a failed path; rotation invalidates every old
permission observation until scopes are revalidated and resources refreshed.
Deleting an authorization removes access bindings while retaining resources,
project attachments, references and operation snapshots.

## Adding a compiled product

1. Implement `catalog.Provider`: `ID`, `InstanceSchema`, `ScopeSchemas`, `Validate`,
   `ValidateScope`, `Products`. Instance and scope locator schemas are provider-defined
   closed public documents; multi-endpoint products do not need core changes.
   Return an allowlisted public principal. Scope locators must be public
   projections; context is a capability map, not raw OAuth response metadata.
2. A product declares one or more `ResourceTypeDescriptor`s. Give each type a
   stable ID, identity rule, allowed scope types, display category and roles.
   Roles are arbitrary registered strings, not database enums.
3. Supply versioned JSON Schema 2020-12 for locator, remote reference and public
   observation. Persisted object schemas must set `additionalProperties:false`
   throughout. Secret fields and external schema references are rejected.
   Action input may contain temporary product secrets; it is never persisted.
4. Implement `ResourceHandler.Discover` and `Inspect`. Register its view and
   action handlers with `RegisterHandler`. Every declared handler is required;
   undeclared handlers and duplicate IDs fail startup.
5. Declare observed relation contracts with direction, allowed source/target
   types, cardinality, attributes schema and deletion policy. User annotations
   use separately registered `origin=user` relations. `CompleteRelations`
   means an observation is authoritative for those relation types on that one
   access. Omit it for partial results. Unimported parents remain references.
6. Implement `ReferenceResolver` when the provider can prove identity behind a
   typed reference. `POST /resource-references/{id}/resolve` must select an
   appropriate bound scope; the resolver reads the remote object. Locator
   equality alone never resolves references.
7. Register the provider and handlers at startup, then call `ValidateReady`.
   The fixture in `internal/catalogtest` demonstrates a Neon-like product
   with project, branch and database types, plus a scoped webhook. Tests also
   register a second SaaS provider without changing core SQL/services/UI.

`resource_kind` is no longer a service routing primitive. Views and actions
are dispatched by registered IDs. Execution, Deployment and DNSRecord DTOs
remain useful standard view outputs. The UI renders type/action schemas,
access observations and live view results generically.

## Explicit operations

`POST /operations` requires `Idempotency-Key`, a type/action ID, a target kind
and ID, and a versioned input document. Scope targets use a connection-scope
ID; resource targets use a resource ID and selected resource-access ID.

The journal is committed before dispatch. It retains action, target/access
snapshots, credential revision, authorization policy, an HMAC request
fingerprint, status and a validated public result and a separate created-resource ID. Inputs and raw upstream
errors are never written. Same-key replays return the stored result, including
`unknown`; changed payloads conflict. Interrupted writes become `unknown` at
startup. A journal failure after remote success also remains ambiguous; the
service never compensates by blindly deleting or adopting by name.

Actions default to confirmed capabilities. Integrations may declare
`authorization=remote` for non-delete actions when the upstream does not expose
complete token permissions: `unavailable` still denies, while `unknown` permits
one explicit request through the selected credential and records that decision.
Remote deletion always requires a confirmed available capability. Resource
writes inspect the remote identity before dispatch; mismatches fail without
sending the write. Definite denials can be recorded as `failed`; transport and
ambiguous upstream errors are `unknown`.

A remote deletion additionally requires `origin=mevius`, no protection, no
project attachment and no blocking incoming reference. The journal reservation
blocks attachment/protection/authorization changes during dispatch. Protection
does not change origin. Local forgetting preserves remote objects. Ordinary
reimport always starts as external and cannot regain deletion eligibility.

`POST /operations/{id}/check` only reads a previously submitted remote operation
using a registered checker and its original access/credential revision. It
requires a remote operation ID; it never resubmits a write or continuously
reconciles state. GitHub run and deployment receipts use this path. Providers
without a correlated remote operation ID retain their receipt or unknown state.

## Storage and release boundary

All entity IDs are UUIDs; SQLite timestamps are UTC Unix milliseconds; HTTP
uses ISO 8601. Foreign keys, instance-matching triggers, identity uniqueness,
JSON validity, positive versions and status constraints enforce invariants.

`internal/store/baseline/0001_catalog.sql` is the active epoch 6 baseline.
`internal/store/migrations/0001_resource_model.sql` is archived epoch 5 history
and is not executed. Old/non-epoch databases are rejected before migration.
Reset only through the explicit backup-and-reset CLI. After release, add
incremental Goose migrations to `baseline`; never edit or replace the baseline
for existing installations. The store uses explicit parameterized SQL; obsolete
SQLC queries and generated epoch 5 models have been removed.

## Validation map

`internal/catalog/catalog_test.go` verifies canonical encoding, credential
isolation, scope/environment/instance namespaces, independent children and
project aliases, reference resolution, path-specific failures, partial relation
updates, concurrent import transactions, deletion policy, transient secrets,
idempotency and unknown outcomes, schema versions, historical unregistered
snapshots and provider extension. OAuth tests cover PKCE, single atomic session
consumption and secret erasure. Store tests reject old databases without
mutation and exercise FK/JSON/version constraints. CLI tests perform a complete
old-epoch backup/reset. Frontend tests exercise schema rendering, independent
authorization, extensible roles, repeated attachment and key preservation.

## Current provider limits

The fixtures validate Neon-like and SaaS shapes; they are not real Neon/Stripe/
Sentry integrations. OAuth credentials that expire require reauthorization.
GitHub workflow dispatch does not return a correlated run ID, so its submission
receipt cannot be automatically matched to a run. Vercel log reads require the
deployment to appear in that project's visible deployment list; older entries
outside that list are denied rather than reading an unverified deployment.
Runtime outputs are read on demand, and public observations remain allowlisted
projections. No new real provider credentials were used in the test suite.
