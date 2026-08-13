import * as Crypto from 'expo-crypto';
import * as SecureStore from 'expo-secure-store';
import { Platform } from 'react-native';

// Point this at your machine's LAN IP when running on a physical device —
// localhost on a phone means the phone.
export const API_URL =
  process.env.EXPO_PUBLIC_API_URL ??
  (Platform.OS === 'android' ? 'http://10.0.2.2:8080' : 'http://localhost:8080');

const DEVICE_KEY = 'hairpin.device_id';
const TOKEN_KEY = 'hairpin.token';

// SecureStore has no web implementation; fall back to localStorage there so
// `npm run web` works for development.
const storage = {
  async get(key: string): Promise<string | null> {
    if (Platform.OS === 'web') {
      try {
        return globalThis.localStorage?.getItem(key) ?? null;
      } catch {
        return null;
      }
    }
    return SecureStore.getItemAsync(key);
  },
  async set(key: string, value: string): Promise<void> {
    if (Platform.OS === 'web') {
      try {
        globalThis.localStorage?.setItem(key, value);
      } catch {
        /* private browsing — session will not persist */
      }
      return;
    }
    await SecureStore.setItemAsync(key, value);
  },
  async remove(key: string): Promise<void> {
    if (Platform.OS === 'web') {
      try {
        globalThis.localStorage?.removeItem(key);
      } catch {
        /* ignore */
      }
      return;
    }
    await SecureStore.deleteItemAsync(key);
  },
};

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

let token: string | null = null;

/** The device secret IS the account. It is generated once and never leaves the keystore. */
async function deviceId(): Promise<string> {
  const existing = await storage.get(DEVICE_KEY);
  if (existing) return existing;
  const fresh = Crypto.randomUUID() + Crypto.randomUUID().replace(/-/g, '');
  await storage.set(DEVICE_KEY, fresh);
  return fresh;
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(API_URL + path, {
    method,
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });

  const raw = await res.text();
  let parsed: unknown = null;
  if (raw) {
    try {
      parsed = JSON.parse(raw);
    } catch {
      throw new ApiError(res.status, 'bad_json', raw.slice(0, 200));
    }
  }

  if (!res.ok) {
    const e = parsed as { error?: string; message?: string } | null;
    throw new ApiError(res.status, e?.error ?? 'unknown', e?.message ?? `HTTP ${res.status}`);
  }
  return parsed as T;
}

export const api = {
  get: <T,>(path: string) => request<T>('GET', path),
  post: <T,>(path: string, body?: unknown) => request<T>('POST', path, body),

  /**
   * Signs in with the stored device secret, reusing a cached token when one is
   * still valid. A 401 means the token expired or the server rotated its
   * secret — either way, re-registering the same device recovers the account.
   */
  async signIn(): Promise<void> {
    const cached = await storage.get(TOKEN_KEY);
    if (cached) {
      token = cached;
      try {
        await request('GET', '/v1/me');
        return;
      } catch (err) {
        if (!(err instanceof ApiError) || err.status !== 401) throw err;
        token = null;
      }
    }
    const res = await request<{ token: string }>('POST', '/v1/auth/device', {
      device_id: await deviceId(),
    });
    token = res.token;
    await storage.set(TOKEN_KEY, res.token);
  },

  /** Drops the session but keeps the device secret, so the account survives. */
  async signOut(): Promise<void> {
    token = null;
    await storage.remove(TOKEN_KEY);
  },
};
