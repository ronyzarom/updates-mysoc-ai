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

class ApiClient {
  private baseUrl: string;
  private apiKey: string;

  constructor() {
    this.baseUrl = API_URL;
    this.apiKey = "";
  }

  setApiKey(key: string) {
    this.apiKey = key;
  }

  private async fetch<T>(path: string, options: RequestInit = {}): Promise<T> {
    const headers: HeadersInit = {
      "Content-Type": "application/json",
      ...(options.headers || {}),
    };

    if (this.apiKey) {
      (headers as Record<string, string>)["X-API-Key"] = this.apiKey;
    }

    const response = await fetch(`${this.baseUrl}${path}`, {
      ...options,
      headers,
    });

    if (!response.ok) {
      const error = await response.json().catch(() => ({ error: "Unknown error" }));
      throw new Error(error.error || `API error: ${response.status}`);
    }

    return response.json();
  }

  // Instances
  async getInstances(): Promise<Instance[]> {
    return this.fetch<Instance[]>("/api/v1/instances");
  }

  async getInstance(id: string): Promise<Instance> {
    return this.fetch<Instance>(`/api/v1/instances/${id}`);
  }

  async deleteInstance(id: string): Promise<void> {
    await this.fetch(`/api/v1/instances/${id}`, { method: "DELETE" });
  }

  // Licenses
  async getLicenses(): Promise<License[]> {
    return this.fetch<License[]>("/api/v1/admin/licenses");
  }

  async getLicense(id: string): Promise<License> {
    return this.fetch<License>(`/api/v1/admin/licenses/${id}`);
  }

  async createLicense(data: Partial<License>): Promise<License> {
    return this.fetch<License>("/api/v1/admin/licenses", {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async updateLicense(id: string, data: Partial<License>): Promise<License> {
    return this.fetch<License>(`/api/v1/admin/licenses/${id}`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
  }

  async deleteLicense(id: string): Promise<void> {
    await this.fetch(`/api/v1/admin/licenses/${id}`, { method: "DELETE" });
  }

  // Releases
  async getReleases(): Promise<Release[]> {
    return this.fetch<Release[]>("/api/v1/releases");
  }

  async getProductReleases(product: string): Promise<Release[]> {
    return this.fetch<Release[]>(`/api/v1/releases/${product}`);
  }

  // Health
  async getHealth(): Promise<{ status: string; version: string }> {
    return this.fetch("/health");
  }
}

export const api = new ApiClient();

