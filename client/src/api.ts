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
const AUTH_PATH = '/v1/auth/device';

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

/**
 * Sends one request. A 401 means the session token expired or the server
 * rotated its signing secret — the device secret is still good, so we
 * re-register and replay the request once rather than dumping the player back
 * to an error screen. `retry` guards against looping if auth itself 401s.
 */
async function request<T>(method: string, path: string, body?: unknown, retry = true): Promise<T> {
  const res = await fetch(API_URL + path, {
    method,
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });

  if (res.status === 401 && retry && path !== AUTH_PATH) {
    await res.text(); // drain so the connection can be reused
    await registerDevice();
    return request<T>(method, path, body, false);
  }

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

/** Trades the device secret for a fresh session token. */
async function registerDevice(): Promise<void> {
  const res = await request<{ token: string }>(
    'POST',
    AUTH_PATH,
    { device_id: await deviceId() },
    false
  );
  token = res.token;
  await storage.set(TOKEN_KEY, res.token);
}

export const api = {
  get: <T,>(path: string) => request<T>('GET', path),
  post: <T,>(path: string, body?: unknown) => request<T>('POST', path, body),

  /**
   * Restores a cached session, or registers the device if there isn't one.
   * A stale cached token costs nothing to try — the first request that sees a
   * 401 re-registers and replays itself.
   */
  async signIn(): Promise<void> {
    const cached = await storage.get(TOKEN_KEY);
    if (cached) {
      token = cached;
      return;
    }
    await registerDevice();
  },
};
