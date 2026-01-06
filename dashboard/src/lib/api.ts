// Use relative URLs for same-origin requests
const API_URL = "";

export interface Instance {
  id: string;
  instance_id: string;
  instance_type: string;
  hostname: string;
  license_id?: string;
  status: string;
  last_heartbeat?: string;
  last_heartbeat_data?: HeartbeatData;
  created_at: string;
  updated_at: string;
}

export interface HeartbeatData {
  instance_id: string;
  updater_version: string;
  products: ProductStatus[];
  system: SystemMetrics;
  security?: SecurityStatus;
  timestamp: string;
}

export interface ProductStatus {
  name: string;
  version: string;
  status: string;
  health_status?: string;
}

export interface SystemMetrics {
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

export interface License {
  id: string;
  license_key: string;
  customer_id: string;
  customer_name: string;
  type: string;
  products: string[];
  expires_at: string;
  is_active: boolean;
  created_at: string;
}

export interface Release {
  id: string;
  product_name: string;
  version: string;
  channel: string;
  artifact_size: number;
  checksum: string;
  release_notes?: string;
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

  private async fetch<T>(
    path: string,
    options: RequestInit = {},
    requireAuth = false
  ): Promise<T> {
    const headers: HeadersInit = {
      "Content-Type": "application/json",
      ...(options.headers || {}),
    };

    if (this.accessToken && requireAuth) {
      (headers as Record<string, string>)["Authorization"] =
        `Bearer ${this.accessToken}`;
    }

    let response = await fetch(`${this.baseUrl}${path}`, {
      ...options,
      headers,
    });

    // If 401 and we have a refresh token, try to refresh
    if (response.status === 401 && this.refreshToken && requireAuth) {
      const refreshed = await this.refreshTokens();
      if (refreshed) {
        (headers as Record<string, string>)["Authorization"] =
          `Bearer ${this.accessToken}`;
        response = await fetch(`${this.baseUrl}${path}`, {
          ...options,
          headers,
        });
      }
    }

    if (!response.ok) {
      const error = await response
        .json()
        .catch(() => ({ error: "Unknown error" }));
      throw new Error(error.error || `API error: ${response.status}`);
    }

    return response.json();
  }

  // Auth methods
  async login(email: string, password: string): Promise<LoginResponse> {
    const response = await this.fetch<LoginResponse>("/api/v1/auth/login", {
      method: "POST",
      body: JSON.stringify({ email, password }),
    });

    if (!response.requires_mfa && response.access_token && response.refresh_token) {
      this.setTokens(response.access_token, response.refresh_token);
    }

    return response;
  }

  async verifyMFA(mfaToken: string, totpCode: string): Promise<LoginResponse> {
    const response = await this.fetch<LoginResponse>("/api/v1/auth/mfa/verify", {
      method: "POST",
      body: JSON.stringify({ mfa_token: mfaToken, totp_code: totpCode }),
    });

    if (response.access_token && response.refresh_token) {
      this.setTokens(response.access_token, response.refresh_token);
    }

    return response;
  }

  async refreshTokens(): Promise<boolean> {
    if (!this.refreshToken) return false;

    try {
      const response = await fetch(`${this.baseUrl}/api/v1/auth/refresh`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ refresh_token: this.refreshToken }),
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

  async getInstance(id: string): Promise<Instance> {
    return this.fetch<Instance>(`/api/v1/instances/${id}`);
  }

  async deleteInstance(id: string): Promise<void> {
    await this.fetch(`/api/v1/instances/${id}`, { method: "DELETE" }, true);
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
    return this.fetch("/health");
  }
}

export const api = new ApiClient();
