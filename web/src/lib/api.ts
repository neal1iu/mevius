const API_BASE_URL = '/api/v1'
const TOKEN_KEY = 'mevius_token'

export class ApiError extends Error { readonly status:number;constructor(status:number,message:string){super(message);this.status=status;this.name='ApiError'} }
export const getToken=()=>localStorage.getItem(TOKEN_KEY)
export const setToken=(token:string)=>localStorage.setItem(TOKEN_KEY,token)
export const clearToken=()=>localStorage.removeItem(TOKEN_KEY)
type UnauthorizedHandler=()=>void
let unauthorizedHandler:UnauthorizedHandler|null=null
export const setUnauthorizedHandler=(handler:UnauthorizedHandler|null)=>{unauthorizedHandler=handler}

export async function apiFetch<T>(path:string,init?:RequestInit):Promise<T>{const headers=new Headers(init?.headers);headers.set('Accept','application/json');if(init?.body!=null&&!headers.has('Content-Type'))headers.set('Content-Type','application/json');const token=getToken();if(token)headers.set('Authorization',`Bearer ${token}`);const response=await fetch(`${API_BASE_URL}${path}`,{...init,headers});if(response.status===401){clearToken();unauthorizedHandler?.();throw new ApiError(401,'Unauthorized')}if(!response.ok){let message=response.statusText;try{const body=await response.json() as {error?:string;message?:string};message=body.error??body.message??message}catch{}throw new ApiError(response.status,message)}if(response.status===204)return undefined as T;const contentType=response.headers.get('content-type')??'';return (contentType.includes('application/json')?await response.json():await response.text()) as T}

export type CapabilityAvailability='available'|'unavailable'|'unknown'
export interface CapabilityState{availability:CapabilityAvailability;reason?:string}
export interface FieldSchema{name:string;label:string;type:string;required?:boolean;description?:string}
export interface ProductDescriptor{id:string;provider_id:string;display_name:string;resource_kind:string;capabilities:string[];fields?:FieldSchema[]}
export interface ProviderDescriptor{id:string;display_name:string;products:ProductDescriptor[]}
export interface Catalog{providers:ProviderDescriptor[]}
export interface ProviderScope{type:string;id:string;label:string;meta?:Record<string,unknown>}
export interface ProbeResult{identity:Record<string,unknown>;scopes:ProviderScope[];permissions:Record<string,CapabilityState>}
export interface Connection{id:string;provider_id:string;label:string;endpoint?:string;scope:ProviderScope;auth_method:'token'|'oauth';remote_identity?:Record<string,unknown>;permissions?:Record<string,CapabilityState>;created_at:string;updated_at:string}
export interface OAuthProviderInfo{provider_id:string;available:boolean;configured:boolean;source?:'database'|'environment';client_id?:string;authorization_url:string;token_url:string;scopes:string[];pkce:boolean;redirect_base_url?:string;callback_url?:string;requires_slug?:boolean;integration_slug?:string}
export interface OAuthAuthorizationSession{id:string;provider_id:string;endpoint?:string;status:'pending'|'authorized'|'consumed'|'failed';identity?:Record<string,unknown>;scopes?:ProviderScope[];permissions?:Record<string,CapabilityState>;error?:string;expires_at:string}
export interface ResourceInstance{id:string;connection_id:string;provider_product_id:string;resource_kind:string;external_id:string;external_url?:string;display_name:string;lifecycle_mode:'managed'|'imported';spec?:unknown;provider_config?:unknown;cached_meta?:Record<string,unknown>;sync_status:string;capabilities?:Record<string,CapabilityState>;last_synced_at?:string;created_at:string;updated_at:string}
export interface Project{id:string;name:string;description?:string;created_at:string;updated_at:string}
export interface ProjectResource{id:string;project_id:string;resource_instance_id:string;alias:string;purpose?:string;created_at:string;updated_at:string}
export interface ProjectResourceDetail{project_resource:ProjectResource;resource_instance:ResourceInstance}
export interface ResourceRelation{id:string;from_resource_instance_id:string;to_resource_instance_id:string;relation_type:'source_repo'|'deploys_to';origin:'system'|'user';config?:unknown;created_at:string}
export interface ProjectDetail{project:Project;resources:ProjectResourceDetail[]|null;relations:ResourceRelation[]|null}
export interface ExternalResource{external_id:string;external_url?:string;display_name:string;provider_config?:unknown;meta?:Record<string,unknown>;capabilities?:Record<string,CapabilityState>}
export interface Execution{id:string;status:string;provider_status?:string;ref?:string;commit_sha?:string;external_url?:string;created_at?:string;started_at?:string;finished_at?:string}
export interface DNSRecord{id?:string;type:string;name:string;content:string;ttl?:number;proxied?:boolean;priority?:number}

export const idempotencyKey=()=>crypto.randomUUID()

if(import.meta.env.DEV){(window as unknown as {meviusApi?:unknown}).meviusApi={apiFetch,getToken,setToken,clearToken}}
