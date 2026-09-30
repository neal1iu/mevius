import { apiFetch } from "./api";
export interface Document<T = Record<string, unknown>> {
  version: number;
  data: T;
}
export interface Schema {
  version: number;
  schema: Record<string, unknown>;
}
export interface ResourceType {
  id: string;
  product_id: string;
  provider_id: string;
  display_name: string;
  category: string;
  compatible_roles: string[];
  identity: { scope: string; natural: boolean; parent_type?: string };
  scope_types: string[];
  locator: Schema;
  reference: Schema;
  observation: Schema;
  views: View[];
  actions: Action[];
  relations: RelationDescriptor[];
}
export interface View {
  id: string;
  display_name: string;
  input: Schema;
  output: Schema;
  capability: string;
}
export interface Action extends View {
  target: "scope" | "resource";
  effect: "create" | "delete" | "update";
  authorization?: "confirmed" | "remote";
}
export interface RelationDescriptor {
  id: string;
  origin: "user" | "adapter";
  from_types: string[];
  to_types: string[];
  cardinality: string;
  blocks_deletion: boolean;
  attributes: Schema;
}
export interface Catalog {
  providers?: {
    id: string;
    display_name: string;
    instance: Schema;
    scopes: Record<string, Schema>;
  }[];
  products: {
    id: string;
    provider_id: string;
    display_name: string;
    resource_types: ResourceType[];
  }[];
  relations: RelationDescriptor[];
}
export interface Instance {
  id: string;
  provider_id: string;
  instance_key: string;
  endpoint: Document<{ url: string }>;
}
export interface Connection {
  id: string;
  provider_instance_id: string;
  label: string;
  auth_scheme: string;
  credential_revision: number;
  principal: Document;
  authorization_state: string;
}
export interface ScopeCandidate {
  parent?: ScopeCandidate;
  scope_type: string;
  identity_parts: string[];
  locator: Record<string, unknown>;
  label: string;
  context: Record<string, unknown>;
}
export interface Scope {
  id: string;
  provider_instance_id: string;
  scope_type: string;
  scope_key: string;
  locator: Document;
  label: string;
}
export interface Binding {
  id: string;
  connection_id: string;
  scope_id: string;
  context: Document;
  validation_state: string;
}
export interface ScopeResponse {
  available: ScopeCandidate[];
  bindings: Binding[];
  scopes: Scope[];
}
export interface Resource {
  id: string;
  provider_instance_id: string;
  resource_type_id: string;
  identity_key: string;
  identity_version: number;
  scope_id: string;
  locator: Document;
  display_name: string;
  origin: "external" | "mevius" | "unknown";
  delete_protection: boolean;
  created_at: string;
  updated_at: string;
}
export interface Capability {
  availability: "available" | "unavailable" | "unknown";
}
export interface Access {
  connection_id: string;
  connection_label: string;
  scope_label: string;
  validation_state: string;
  current_credential_revision: number;
  id: string;
  resource_id: string;
  connection_scope_id: string;
  credential_revision: number;
  observation: Document;
  capabilities: Record<string, Capability>;
  last_attempt_at: string;
  last_success_at?: string;
  error_code?: string;
}
export interface Reference {
  id?: string;
  provider_id: string;
  provider_instance_id?: string;
  resource_type_id: string;
  remote: Document;
  identity_parts?: string[];
  resolved_resource_id?: string;
}
export interface Relation {
  id: string;
  from_resource_id: string;
  relation_type: string;
  origin: string;
  reference: Reference;
  attributes: Document;
  blocks_deletion: boolean;
  observed_access_id?: string;
}
export interface ResourceDetail {
  resource: Resource;
  accesses: Access[];
  selected_access?: Access;
  relations: Relation[];
}
export interface Observation {
  identity_parts: string[];
  locator: Record<string, unknown>;
  display_name: string;
  observation: Record<string, unknown>;
  capabilities: Record<string, Capability>;
}
export interface OperationInput {
  resource_type_id: string;
  action_id: string;
  target_kind: "scope" | "resource";
  target_id: string;
  access_id?: string;
  input: Document;
}
export interface Operation {
  result_resource_id?: string;
  id: string;
  action_id: string;
  resource_type_id: string;
  target_kind: string;
  target_id: string;
  access_id: string;
  connection_id: string;
  credential_revision: number;
  snapshot: Document;
  status: "in_progress" | "succeeded" | "failed" | "unknown";
  result: Document;
  error_code?: string;
  remote_operation_id?: string;
  created_at: string;
}
export interface Project {
  id: string;
  name: string;
  description: string;
}
export interface Attachment {
  id: string;
  project_id: string;
  resource_id: string;
  alias: string;
  role: string;
  purpose: string;
  environment: string;
}
export interface OAuthInfo {
  provider_id: string;
  configured: boolean;
  configuration: {
    client_id: string;
    authorization_url: string;
    token_url: string;
    scopes: string[];
    pkce: boolean;
    redirect_base_url: string;
  };
}
export const document = (
  data: Record<string, unknown>,
  version = 1,
): Document => ({ version, data });
export const types = (catalog?: Catalog) =>
  catalog?.products.flatMap((p) => p.resource_types) ?? [];
export const send = <T>(
  path: string,
  data: unknown,
  method = "POST",
  headers?: HeadersInit,
) => apiFetch<T>(path, { method, body: JSON.stringify(data), headers });
// Preserve the key across HTTP errors and page reloads. Replaying an unknown result
// reads the journal; it does not execute the action again.
export async function operationKey(input: OperationInput): Promise<string> {
  const digest = await crypto.subtle.digest(
    "SHA-256",
    new TextEncoder().encode(JSON.stringify(input)),
  );
  const fingerprint = Array.from(new Uint8Array(digest), (value) =>
    value.toString(16).padStart(2, "0"),
  ).join("");
  const name = `mevius:operation:${fingerprint}`;
  let key = sessionStorage.getItem(name);
  if (!key) {
    key = crypto.randomUUID();
    sessionStorage.setItem(name, key);
  }
  return key;
}
export const submitOperation = async (input: OperationInput) =>
  send<Operation>("/operations", input, "POST", {
    "Idempotency-Key": await operationKey(input),
  });
