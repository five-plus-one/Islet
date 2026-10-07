// 统一 API 客户端与全部端点封装

export interface ApiResponse<T> {
  code: number;
  message: string;
  data: T;
}

export class ApiError extends Error {
  code: number;
  status: number;
  constructor(message: string, code: number, status: number) {
    super(message);
    this.code = code;
    this.status = status;
  }
}

export async function request<T>(path: string, options: RequestInit = {}, auth = true): Promise<T> {
  const headers = new Headers(options.headers);
  if (auth) {
    const token = getAccessToken();
    if (token) headers.set('Authorization', `Bearer ${token}`);
  }
  if (options.body && !headers.has('Content-Type') && !(options.body instanceof FormData)) {
    headers.set('Content-Type', 'application/json');
  }
  const response = await fetch(path, { ...options, headers });
  let payload: ApiResponse<T> | null = null;
  try {
    payload = (await response.json()) as ApiResponse<T>;
  } catch {
    payload = null;
  }
  if (!response.ok || !payload || payload.code !== 0) {
    const message = payload?.message ?? `请求失败（HTTP ${response.status}）`;
    throw new ApiError(message, payload?.code ?? response.status, response.status);
  }
  return payload.data;
}

// ---- 会话 token 存储 ----

const accessTokenKey = 'islet_access_token';
const idTokenKey = 'islet_id_token';
const verifierKey = 'islet_pkce_verifier';
const stateKey = 'islet_oidc_state';

export function getAccessToken(): string {
  return localStorage.getItem(accessTokenKey) ?? '';
}

export function getIDToken(): string {
  return localStorage.getItem(idTokenKey) ?? '';
}

export function saveSession(accessToken: string, idToken: string): void {
  localStorage.setItem(accessTokenKey, accessToken);
  if (idToken) localStorage.setItem(idTokenKey, idToken);
}

export function clearSession(): void {
  localStorage.removeItem(accessTokenKey);
  localStorage.removeItem(idTokenKey);
  sessionStorage.removeItem(verifierKey);
  sessionStorage.removeItem(stateKey);
}

// ---- 类型 ----

export interface AuthConfig {
  mode: 'oidc' | 'local';
  local_enabled: boolean;
  issuer: string;
  client_id: string;
  scopes: string;
  authorization_endpoint: string;
  token_endpoint: string;
  redirect_uri: string;
  site_title: string;
  brand_name: string;
  brand_logo: string;
  oidc_label: string;
  oidc_logo: string;
}

export interface AuthUser {
  id: number;
  subject: string;
  email: string;
  username: string;
  role: 'super_admin' | 'admin' | 'user';
  status: string;
  auth_source: 'oidc' | 'local';
  group_id: number;
  via_token?: boolean;
}

export interface PolicyView {
  id: number;
  name: string;
  driver: 'local' | 's3';
  url_mode: 'direct' | 'proxy';
  naming: 'keep' | 'random' | 'date';
  is_default: boolean;
  status: string;
  endpoint?: string;
  region?: string;
  bucket?: string;
  prefix?: string;
  cdn?: string;
  access_key?: string;
  path_style?: boolean;
  secret_configured: boolean;
}

export interface PolicyInput {
  id?: number;
  name: string;
  driver: string;
  url_mode: string;
  naming: string;
  is_default: boolean;
  status: string;
  endpoint: string;
  region: string;
  bucket: string;
  prefix: string;
  cdn: string;
  access_key: string;
  secret_key: string;
  path_style: boolean;
}

export interface GroupView {
  id: number;
  name: string;
  description: string;
  quota_bytes: number;
  max_file_bytes: number;
  allowed_mimes: string[];
  policy_ids: number[];
  uploads_per_hour: number;
  built_in: boolean;
  status: string;
  members?: number;
}

export interface UserView {
  id: number;
  username: string;
  email: string;
  auth_source: string;
  role: string;
  status: string;
  group_id: number;
  group_name?: string;
  last_login_at?: string | null;
  created_at: string;
}

export interface ImageItem {
  id: number;
  user_id: number;
  username?: string;
  policy_id: number;
  album_id?: number | null;
  url: string;
  thumb_url: string;
  original_name: string;
  mime: string;
  size: number;
  width: number;
  height: number;
  tags: string[];
  created_at: string;
  deleted_at?: string | null;
}

export interface TokenView {
  id: number;
  name: string;
  expires_at?: string | null;
  last_used_at?: string | null;
  status: string;
  created_at: string;
}

export interface AlbumView {
  id: number;
  name: string;
  description: string;
  image_count?: number;
  created_at: string;
}

export interface MeInfo {
  user: AuthUser;
  group: Partial<GroupView>;
  quota: { limit: number; used: number };
  policies: PolicyView[];
}

// ---- API 函数 ----

export const api = {
  authConfig: () => request<AuthConfig>('/api/v1/auth/config', {}, false),

  localLogin: (username: string, password: string) =>
    request<AuthUser>('/api/v1/auth/local/login', { method: 'POST', body: JSON.stringify({ username, password }) }, false),

  oidcExchange: (code: string, verifier: string, redirectUri: string) =>
    request<{ access_token: string; id_token: string; expires_in: number; username: string }>(
      '/api/v1/auth/oidc/callback',
      { method: 'POST', body: JSON.stringify({ code, code_verifier: verifier, redirect_uri: redirectUri }) },
      false
    ),

  logout: () => request<{ logged_out: boolean }>('/api/v1/auth/logout', { method: 'POST' }),

  me: () => request<AuthUser>('/api/v1/auth/me'),

  meInfo: () => request<MeInfo>('/api/v1/me'),

  tokens: () => request<{ items: TokenView[] }>('/api/v1/me/tokens'),
  createToken: (name: string, expiresAt?: string) =>
    request<{ token: string; item: TokenView }>('/api/v1/me/tokens', {
      method: 'POST',
      body: JSON.stringify({ name, expires_at: expiresAt || null })
    }),
  revokeToken: (id: number) => request<{ revoked: boolean }>(`/api/v1/me/tokens/${id}`, { method: 'DELETE' }),

  images: (params: string) => request<{ items: ImageItem[] }>(`/api/v1/images${params}`),
  updateImage: (id: number, body: object) => request<ImageItem>(`/api/v1/images/${id}`, { method: 'PATCH', body: JSON.stringify(body) }),
  recycleImage: (id: number) => request<{ recycled: boolean }>(`/api/v1/images/${id}`, { method: 'DELETE' }),
  restoreImage: (id: number) => request<{ restored: boolean }>(`/api/v1/images/${id}/restore`, { method: 'POST' }),
  purgeImage: (id: number) => request<{ purged: boolean }>(`/api/v1/images/${id}/purge`, { method: 'DELETE' }),

  albums: () => request<{ items: AlbumView[] }>('/api/v1/albums'),
  createAlbum: (name: string, description: string) =>
    request<AlbumView>('/api/v1/albums', { method: 'POST', body: JSON.stringify({ name, description }) }),
  updateAlbum: (id: number, name: string, description: string) =>
    request<AlbumView>(`/api/v1/albums/${id}`, { method: 'PATCH', body: JSON.stringify({ name, description }) }),
  deleteAlbum: (id: number) => request<{ deleted: boolean }>(`/api/v1/albums/${id}`, { method: 'DELETE' }),

  policies: () => request<{ items: PolicyView[] }>('/api/admin/storage-policies'),
  savePolicy: (input: PolicyInput) =>
    input.id
      ? request<PolicyView>(`/api/admin/storage-policies/${input.id}`, { method: 'PATCH', body: JSON.stringify(input) })
      : request<PolicyView>('/api/admin/storage-policies', { method: 'POST', body: JSON.stringify(input) }),
  deletePolicy: (id: number) => request<{ deleted: boolean }>(`/api/admin/storage-policies/${id}`, { method: 'DELETE' }),
  testPolicy: (input: Partial<PolicyInput>) =>
    request<{ ok: boolean; message: string }>('/api/admin/storage-policies/test', { method: 'POST', body: JSON.stringify(input) }),
  testPolicyById: (id: number) =>
    request<{ ok: boolean; message: string }>(`/api/admin/storage-policies/${id}/test`, { method: 'POST' }),

  groups: () => request<{ items: GroupView[] }>('/api/admin/user-groups'),
  saveGroup: (input: Partial<GroupView> & { name: string }) =>
    input.id
      ? request<GroupView>(`/api/admin/user-groups/${input.id}`, { method: 'PATCH', body: JSON.stringify(input) })
      : request<GroupView>('/api/admin/user-groups', { method: 'POST', body: JSON.stringify(input) }),
  deleteGroup: (id: number) => request<{ deleted: boolean }>(`/api/admin/user-groups/${id}`, { method: 'DELETE' }),

  users: (params = '') => request<{ items: UserView[] }>(`/api/admin/users${params}`),
  setUserGroup: (id: number, groupId: number) =>
    request<{ updated: boolean }>(`/api/admin/users/${id}/group`, { method: 'PATCH', body: JSON.stringify({ group_id: groupId }) }),
  setUserRole: (id: number, role: string) =>
    request<{ updated: boolean }>(`/api/admin/users/${id}/role`, { method: 'PATCH', body: JSON.stringify({ role }) }),
  setUserStatus: (id: number, status: string) =>
    request<{ updated: boolean }>(`/api/admin/users/${id}/status`, { method: 'PATCH', body: JSON.stringify({ status }) }),

  overview: () => request<Record<string, unknown>>('/api/admin/stats/overview'),
  trend: (days: number) => request<{ items: { date: string; count: number; bytes: number }[] }>(`/api/admin/stats/trend?days=${days}`),
  groupUsage: () => request<{ items: { group_id: number; name: string; quota_bytes: number; used_bytes: number; images: number }[] }>('/api/admin/stats/groups'),

  settings: () => request<Record<string, string>>('/api/admin/settings'),
  saveSettings: (values: Record<string, string>) =>
    request<Record<string, string>>('/api/admin/settings', { method: 'PUT', body: JSON.stringify(values) }),
  adminTestOIDC: (issuer: string) =>
    request<{ issuer: string; authorization_endpoint: string; token_endpoint: string; jwks_uri: string }>(
      '/api/admin/auth/test-oidc',
      { method: 'POST', body: JSON.stringify({ issuer }) }
    ),

  recycleList: () => request<{ items: ImageItem[] }>('/api/admin/recycle'),
  purgeAllRecycle: () => request<{ purged: number }>('/api/admin/recycle/purge-all', { method: 'POST' }),

  auditLogs: () => request<{ items: Record<string, unknown>[] }>('/api/admin/audit-logs'),

  resetSystem: (confirm: string, password: string) =>
    request<{ reset: boolean }>('/api/admin/system/reset', { method: 'POST', body: JSON.stringify({ confirm, password }) }),

  setupStatus: () =>
    request<{ status: string; auth_mode: string; admin_verified: boolean; defaults: Record<string, string>; auth_config: AuthConfig }>(
      '/api/setup/status',
      {},
      false
    ),
  setupTestDatabase: (input: Record<string, unknown>) =>
    request<{ mysql: boolean; redis: boolean }>('/api/setup/test/database', { method: 'POST', body: JSON.stringify(input) }, false),
  setupTestOIDC: (issuer: string, clientId: string) =>
    request<{ issuer: string; authorization_endpoint: string; token_endpoint: string }>(
      '/api/setup/test/oidc',
      { method: 'POST', body: JSON.stringify({ issuer, client_id: clientId }) },
      false
    ),
  setupVerifyAdmin: (input: Record<string, string>) =>
    request<{ mode: string; username: string }>('/api/setup/verify/admin', { method: 'POST', body: JSON.stringify(input) }, false),
  setupComplete: (database: Record<string, unknown>, siteTitle: string, publicBaseUrl: string) =>
    request<{ owner: string; status: string }>('/api/setup/complete', {
      method: 'POST',
      body: JSON.stringify({ database, site_title: siteTitle, public_base_url: publicBaseUrl })
    }, false)
};

// ---- OIDC PKCE（纯前端 S256） ----

function randomString(length: number): string {
  const bytes = new Uint8Array(length);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
}

async function pkceChallenge(verifier: string): Promise<string> {
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(verifier));
  return btoa(String.fromCharCode(...new Uint8Array(digest)))
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=+$/, '');
}

export async function beginOIDCLogin(config: AuthConfig, redirectUri?: string): Promise<void> {
  const verifier = randomString(32);
  const state = randomString(16);
  const challenge = await pkceChallenge(verifier);
  sessionStorage.setItem(verifierKey, verifier);
  sessionStorage.setItem(stateKey, state);
  const params = new URLSearchParams({
    response_type: 'code',
    client_id: config.client_id,
    redirect_uri: redirectUri || config.redirect_uri,
    scope: config.scopes || 'openid profile email',
    state,
    code_challenge: challenge,
    code_challenge_method: 'S256'
  });
  window.location.href = `${config.authorization_endpoint}?${params.toString()}`;
}

export function consumeOIDCState(): { state: string; verifier: string } | null {
  const state = new URLSearchParams(window.location.search).get('state');
  const verifier = sessionStorage.getItem(verifierKey) ?? '';
  const savedState = sessionStorage.getItem(stateKey) ?? '';
  if (!state || !verifier || state !== savedState) return null;
  return { state, verifier };
}
