const API_BASE = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';

export interface User {
  id: number;
  name: string;
  displayName?: string;
  email?: string;
  profilePicUrl?: string;
}

export interface Node {
  id: number;
  name: string;
  givenName?: string;
  machineKey?: string;
  nodeKey?: string;
}

export interface ApiKey {
  id: number;
  prefix: string;
  createdAt?: string;
}

export interface PreAuthKey {
  id: number;
  key: string;
  reusable: boolean;
  ephemeral: boolean;
  used: boolean;
}

class ApiClient {
  private baseUrl: string;

  constructor(baseUrl: string = API_BASE) {
    this.baseUrl = baseUrl;
  }

  private async fetch<T>(path: string, options?: RequestInit): Promise<T> {
    const res = await fetch(`${this.baseUrl}${path}`, {
      ...options,
      headers: {
        'Content-Type': 'application/json',
        ...options?.headers,
      },
    });

    if (!res.ok) {
      throw new Error(`API error: ${res.status}`);
    }

    return res.json();
  }

  // Users
  async listUsers(): Promise<{ users: User[] }> {
    return this.fetch('/api/v1/user');
  }

  async createUser(data: { name: string; displayName?: string; email?: string }): Promise<{ user: User }> {
    return this.fetch('/api/v1/user', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  async deleteUser(id: number): Promise<void> {
    return this.fetch(`/api/v1/user/${id}`, { method: 'DELETE' });
  }

  // Nodes
  async listNodes(): Promise<{ nodes: Node[] }> {
    return this.fetch('/api/v1/node');
  }

  async getNode(id: number): Promise<{ node: Node }> {
    return this.fetch(`/api/v1/node/${id}`);
  }

  async deleteNode(id: number): Promise<void> {
    return this.fetch(`/api/v1/node/${id}`, { method: 'DELETE' });
  }

  // API Keys
  async listApiKeys(): Promise<{ apiKeys: ApiKey[] }> {
    return this.fetch('/api/v1/apikey');
  }

  async createApiKey(): Promise<{ apiKey: string }> {
    return this.fetch('/api/v1/apikey', { method: 'POST' });
  }

  async deleteApiKey(prefix: string): Promise<void> {
    return this.fetch(`/api/v1/apikey/${prefix}`, { method: 'DELETE' });
  }

  // PreAuth Keys
  async listPreAuthKeys(): Promise<{ preAuthKeys: PreAuthKey[] }> {
    return this.fetch('/api/v1/preauthkey');
  }

  async createPreAuthKey(data: { reusable: boolean; ephemeral: boolean }): Promise<{ preAuthKey: PreAuthKey }> {
    return this.fetch('/api/v1/preauthkey', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  async deletePreAuthKey(key: string): Promise<void> {
    return this.fetch(`/api/v1/preauthkey?key=${key}`, { method: 'DELETE' });
  }
}

export const api = new ApiClient();