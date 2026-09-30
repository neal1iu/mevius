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
