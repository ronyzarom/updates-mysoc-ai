// Honor an explicit API base URL (e.g. for local dev against a separate API
// port). Defaults to "" so production stays same-origin/relative.
const API_URL = process.env.NEXT_PUBLIC_API_URL || "";

export interface Instance {
  id: string;
  instance_id: string;
  instance_type: string;
  hostname: string;
  display_name?: string;
  license_id?: string;
  status: string;
  auto_update_enabled: boolean;
  update_group: string;
  last_heartbeat?: string;
  last_heartbeat_data?: HeartbeatData;
  // IP address tracking
  last_ip_address?: string;
  last_ip_seen_at?: string;
  // Update attempt tracking
  last_update_from_version?: string;
  last_update_target_version?: string;
  last_update_success?: boolean;
  last_update_error?: string;
  last_update_at?: string;
  created_at: string;
  updated_at: string;
}

export interface LicenseStatus {
  key?: string;
  valid: boolean;
  expires_at?: string;
  last_check?: string;
}

export interface UpdateAttempt {
  from_version: string;
  target_version: string;
  success: boolean;
  error?: string;
  timestamp: string;
}

// Paginated response for instances list
export interface InstancesPagedResponse {
  items: Instance[];
  limit: number;
  offset: number;
  total: number;
}

export interface HeartbeatData {
  instance_id: string;
  updater_version: string;
  products: ProductStatus[];
  system: SystemMetrics;
  security?: SecurityStatus;
  license?: LicenseStatus;
  last_update_attempt?: UpdateAttempt;
  timestamp: string;
}

export interface ProductStatus {
  name: string;
  version: string;
  status: string;
  health_status?: string;
}

export interface SystemMetrics {
  os?: string;
  arch?: string;
  cpu_usage: number;
  memory_total: number;
  memory_used: number;
  disk_total: number;
  disk_used: number;
  load_average: number;
  uptime: number;
}

export interface SecurityStatus {
  firewall_enabled: boolean;
  ssh_hardened: boolean;
  security_score: number;
  pending_updates: number;
  security_updates: number;
  reboot_required: boolean;
}

// License types supported by the server (see pkg/types License.Type).
export type LicenseType = "mysoc-cloud" | "siemcore" | "siemcore-lite";

export const LICENSE_TYPES: LicenseType[] = [
  "mysoc-cloud",
  "siemcore",
  "siemcore-lite",
];

export interface License {
  id: string;
  license_key: string;
  customer_id: string;
  customer_name: string;
  type: LicenseType | string;
  products: string[];
  features?: string[];
  issued_at?: string;
  expires_at: string;
  bound_to?: string;
  is_active: boolean;
  created_at: string;
  updated_at?: string;
}

export interface Release {
  id: string;
  product_name: string;
  version: string;
  channel: string;
  artifact_size: number;
  checksum: string;
  release_notes?: string;
  target_groups?: string[];
  released_at: string;
}

// Auth types
export interface User {
  id: string;
  email: string;
  name: string;
  role: string;
  avatar_url?: string;
  mfa_enabled: boolean;
  is_active: boolean;
  email_verified: boolean;
  last_login_at?: string;
  password_changed_at: string;
  created_at: string;
  updated_at: string;
}

export interface LoginRequest {
  email: string;
  password: string;
}

export interface LoginResponse {
  requires_mfa: boolean;
  mfa_token?: string;
  access_token?: string;
  refresh_token?: string;
  user?: User;
  expires_in?: number;
}

export interface MFAVerifyRequest {
  mfa_token: string;
  totp_code: string;
}

export interface MFASetupResponse {
  secret: string;
  qr_code_url: string;
  qr_code_data: string;
}

export interface MFABackupCodesResponse {
  backup_codes: string[];
}

export interface Session {
  id: string;
  user_id: string;
  user_agent?: string;
  ip_address?: string;
  expires_at: string;
  created_at: string;
}

export interface AuditEvent {
  id: string;
  user_id?: string;
  event_type: string;
  ip_address?: string;
  user_agent?: string;
  details?: Record<string, unknown>;
  created_at: string;
}

class ApiClient {
  private baseUrl: string;
  private accessToken: string | null = null;
  private refreshToken: string | null = null;
  // Single-flight guard so concurrent 401s trigger exactly one token refresh.
  private refreshPromise: Promise<boolean> | null = null;

  constructor() {
    this.baseUrl = API_URL;
    // Load tokens from localStorage if available
    if (typeof window !== "undefined") {
      this.accessToken = localStorage.getItem("access_token");
      this.refreshToken = localStorage.getItem("refresh_token");
    }
  }

  setTokens(accessToken: string, refreshToken: string) {
    this.accessToken = accessToken;
    this.refreshToken = refreshToken;
    if (typeof window !== "undefined") {
      localStorage.setItem("access_token", accessToken);
      localStorage.setItem("refresh_token", refreshToken);
    }
  }

  clearTokens() {
    this.accessToken = null;
    this.refreshToken = null;
    if (typeof window !== "undefined") {
      localStorage.removeItem("access_token");
      localStorage.removeItem("refresh_token");
    }
  }

  isAuthenticated(): boolean {
    return !!this.accessToken;
  }

  // fetch performs an authenticated request with automatic, single-flight token
  // refresh on 401. Requests are authenticated by default; pass requireAuth =
  // false only for genuinely public endpoints (login, health, token refresh).
  // FormData bodies are sent as-is so the browser can set the multipart
  // Content-Type boundary.
  private async fetch<T>(
    path: string,
    options: RequestInit = {},
    requireAuth = true
  ): Promise<T> {
    const isFormData =
      typeof FormData !== "undefined" && options.body instanceof FormData;

    const buildHeaders = (): Record<string, string> => {
      const headers: Record<string, string> = {
        ...(options.headers as Record<string, string> | undefined),
      };
      if (!isFormData && !("Content-Type" in headers)) {
        headers["Content-Type"] = "application/json";
      }
      if (requireAuth && this.accessToken) {
        headers["Authorization"] = `Bearer ${this.accessToken}`;
      }
      return headers;
    };

    let response = await fetch(`${this.baseUrl}${path}`, {
      ...options,
      headers: buildHeaders(),
    });

    // If unauthorized and we can refresh, refresh once (shared) and retry.
    if (response.status === 401 && requireAuth && this.refreshToken) {
      const refreshed = await this.refreshTokens();
      if (refreshed) {
        response = await fetch(`${this.baseUrl}${path}`, {
          ...options,
          headers: buildHeaders(),
        });
      }
    }

    if (!response.ok) {
      const error = await response
        .json()
        .catch(() => ({ error: "Unknown error" }));
      throw new Error(error.error || `API error: ${response.status}`);
    }

    // Some endpoints (e.g. 204) return no body.
    if (response.status === 204) {
      return undefined as T;
    }
    const text = await response.text();
    return (text ? JSON.parse(text) : undefined) as T;
  }

  // Auth methods
  async login(email: string, password: string): Promise<LoginResponse> {
    const response = await this.fetch<LoginResponse>(
      "/api/v1/auth/login",
      {
        method: "POST",
        body: JSON.stringify({ email, password }),
      },
      false
    );

    if (!response.requires_mfa && response.access_token && response.refresh_token) {
      this.setTokens(response.access_token, response.refresh_token);
    }

    return response;
  }

  async verifyMFA(mfaToken: string, totpCode: string): Promise<LoginResponse> {
    const response = await this.fetch<LoginResponse>(
      "/api/v1/auth/mfa/verify",
      {
        method: "POST",
        body: JSON.stringify({ mfa_token: mfaToken, totp_code: totpCode }),
      },
      false
    );

    if (response.access_token && response.refresh_token) {
      this.setTokens(response.access_token, response.refresh_token);
    }

    return response;
  }

  async refreshTokens(): Promise<boolean> {
    if (!this.refreshToken) return false;
    // Coalesce concurrent refreshes: rotating the refresh token more than once
    // in parallel would invalidate sessions and log the user out.
    if (this.refreshPromise) return this.refreshPromise;

    this.refreshPromise = this.doRefresh().finally(() => {
      this.refreshPromise = null;
    });
    return this.refreshPromise;
  }

  private async doRefresh(): Promise<boolean> {
    const refreshToken = this.refreshToken;
    if (!refreshToken) return false;

    try {
      const response = await fetch(`${this.baseUrl}/api/v1/auth/refresh`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ refresh_token: refreshToken }),
      });

      if (!response.ok) {
        this.clearTokens();
        return false;
      }

      const data = await response.json();
      this.setTokens(data.access_token, data.refresh_token);
      return true;
    } catch {
      this.clearTokens();
      return false;
    }
  }

  async logout(): Promise<void> {
    try {
      await this.fetch(
        "/api/v1/auth/logout",
        {
          method: "POST",
          body: JSON.stringify({ refresh_token: this.refreshToken }),
        },
        true
      );
    } finally {
      this.clearTokens();
    }
  }

  async logoutAll(): Promise<void> {
    try {
      await this.fetch("/api/v1/auth/logout-all", { method: "POST" }, true);
    } finally {
      this.clearTokens();
    }
  }

  async getProfile(): Promise<User> {
    return this.fetch<User>("/api/v1/auth/profile", {}, true);
  }

  async updateProfile(name: string, avatarUrl?: string): Promise<User> {
    return this.fetch<User>(
      "/api/v1/auth/profile",
      {
        method: "PUT",
        body: JSON.stringify({ name, avatar_url: avatarUrl }),
      },
      true
    );
  }

  async changePassword(
    currentPassword: string,
    newPassword: string
  ): Promise<void> {
    await this.fetch(
      "/api/v1/auth/password",
      {
        method: "POST",
        body: JSON.stringify({
          current_password: currentPassword,
          new_password: newPassword,
        }),
      },
      true
    );
  }

  async setupMFA(): Promise<MFASetupResponse> {
    return this.fetch<MFASetupResponse>("/api/v1/auth/mfa/setup", {}, true);
  }

  async enableMFA(totpCode: string): Promise<MFABackupCodesResponse> {
    return this.fetch<MFABackupCodesResponse>(
      "/api/v1/auth/mfa/enable",
      {
        method: "POST",
        body: JSON.stringify({ totp_code: totpCode }),
      },
      true
    );
  }

  async disableMFA(password: string, totpCode: string): Promise<void> {
    await this.fetch(
      "/api/v1/auth/mfa/disable",
      {
        method: "POST",
        body: JSON.stringify({ password, totp_code: totpCode }),
      },
      true
    );
  }

  async getSessions(): Promise<Session[]> {
    return this.fetch<Session[]>("/api/v1/auth/sessions", {}, true);
  }

  async getAuditLog(): Promise<AuditEvent[]> {
    return this.fetch<AuditEvent[]>("/api/v1/auth/audit", {}, true);
  }

  // Instances
  async getInstances(): Promise<Instance[]> {
    return this.fetch<Instance[]>("/api/v1/instances");
  }

  // Paginated instances response
  async getInstancesPaged(limit = 50, offset = 0): Promise<InstancesPagedResponse> {
    return this.fetch<InstancesPagedResponse>(
      `/api/v1/instances/paged?limit=${limit}&offset=${offset}`
    );
  }

  async getInstance(id: string): Promise<Instance> {
    return this.fetch<Instance>(`/api/v1/instances/${id}`);
  }

  async deleteInstance(id: string): Promise<void> {
    await this.fetch(`/api/v1/instances/${id}`, { method: "DELETE" }, true);
  }

  async updateInstance(id: string, data: { display_name?: string; auto_update_enabled?: boolean; update_group?: string }): Promise<Instance> {
    return this.fetch<Instance>(
      `/api/v1/instances/${id}`,
      {
        method: "PUT",
        body: JSON.stringify(data),
      },
      true
    );
  }

  async setInstanceAutoUpdate(id: string, enabled: boolean): Promise<void> {
    await this.fetch(`/api/v1/instances/${id}/auto-update`, {
      method: "PUT",
      body: JSON.stringify({ enabled }),
    });
  }

  async setInstanceUpdateGroup(id: string, group: string): Promise<void> {
    await this.fetch(`/api/v1/instances/${id}/update-group`, {
      method: "PUT",
      body: JSON.stringify({ group }),
    });
  }

  // Licenses
  async getLicenses(): Promise<License[]> {
    return this.fetch<License[]>("/api/v1/admin/licenses");
  }

  async getLicense(id: string): Promise<License> {
    return this.fetch<License>(`/api/v1/admin/licenses/${id}`);
  }

  async createLicense(data: Partial<License>): Promise<License> {
    return this.fetch<License>(
      "/api/v1/admin/licenses",
      {
        method: "POST",
        body: JSON.stringify(data),
      },
      true
    );
  }

  async updateLicense(id: string, data: Partial<License>): Promise<License> {
    return this.fetch<License>(
      `/api/v1/admin/licenses/${id}`,
      {
        method: "PUT",
        body: JSON.stringify(data),
      },
      true
    );
  }

  async deleteLicense(id: string): Promise<void> {
    await this.fetch(`/api/v1/admin/licenses/${id}`, { method: "DELETE" }, true);
  }

  // Releases
  async getReleases(): Promise<Release[]> {
    return this.fetch<Release[]>("/api/v1/releases");
  }

  async getProductReleases(product: string): Promise<Release[]> {
    return this.fetch<Release[]>(`/api/v1/releases/${product}`);
  }

  async uploadRelease(data: {
    product: string;
    version: string;
    channel: string;
    release_notes?: string;
    target_groups?: string[];
    artifact: File;
  }): Promise<Release> {
    const formData = new FormData();
    formData.append("product", data.product);
    formData.append("version", data.version);
    formData.append("channel", data.channel);
    if (data.release_notes) {
      formData.append("release_notes", data.release_notes);
    }
    if (data.target_groups && data.target_groups.length > 0) {
      formData.append("target_groups", data.target_groups.join(","));
    }
    formData.append("artifact", data.artifact);

    // Route through the shared request path so uploads get the same auth,
    // single-flight refresh, and error handling as every other call.
    return this.fetch<Release>(
      "/api/v1/releases",
      {
        method: "POST",
        body: formData,
      },
      true
    );
  }

  async deleteRelease(product: string, version: string): Promise<void> {
    await this.fetch(`/api/v1/releases/${product}/${version}`, { method: "DELETE" }, true);
  }

  async updateRelease(product: string, version: string, data: { release_notes?: string; target_groups?: string[] }): Promise<void> {
    await this.fetch(
      `/api/v1/releases/${product}/${version}`,
      {
        method: "PUT",
        body: JSON.stringify(data),
      },
      true
    );
  }

  async updateReleaseTargetGroups(
    product: string,
    version: string,
    targetGroups: string[]
  ): Promise<void> {
    await this.fetch(
      `/api/v1/releases/${product}/${version}/target-groups`,
      {
        method: "PUT",
        body: JSON.stringify({ target_groups: targetGroups }),
      },
      true
    );
  }

  // Admin - Users
  async getUsers(): Promise<User[]> {
    return this.fetch<User[]>("/api/v1/admin/users", {}, true);
  }

  async createUser(data: {
    email: string;
    password: string;
    name: string;
    role: string;
  }): Promise<User> {
    return this.fetch<User>(
      "/api/v1/admin/users",
      {
        method: "POST",
        body: JSON.stringify(data),
      },
      true
    );
  }

  async updateUser(
    id: string,
    data: { name?: string; role?: string; is_active?: boolean }
  ): Promise<User> {
    return this.fetch<User>(
      `/api/v1/admin/users/${id}`,
      {
        method: "PUT",
        body: JSON.stringify(data),
      },
      true
    );
  }

  async deleteUser(id: string): Promise<void> {
    await this.fetch(`/api/v1/admin/users/${id}`, { method: "DELETE" }, true);
  }

  // Health
  async getHealth(): Promise<{ status: string; version: string }> {
    return this.fetch("/health", {}, false);
  }
}

export const api = new ApiClient();
