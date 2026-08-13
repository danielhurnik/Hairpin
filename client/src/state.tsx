import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { ApiError, api } from './api';
import type { GameState } from './types';

type Ctx = {
  state: GameState | null;
  loading: boolean;
  error: string | null;
  /** Re-reads the authoritative state bundle from the server. */
  refresh: () => Promise<void>;
  /** POSTs, then refreshes. Returns the response so callers can show rewards. */
  act: <T,>(path: string, body?: unknown) => Promise<T>;
  retry: () => void;
};

const GameContext = createContext<Ctx | null>(null);

export function GameProvider({ children }: { children: React.ReactNode }) {
  const [state, setState] = useState<GameState | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [nonce, setNonce] = useState(0);

  const refresh = useCallback(async () => {
    const next = await api.get<GameState>('/v1/state');
    setState(next);
  }, []);

  const act = useCallback(
    async <T,>(path: string, body?: unknown): Promise<T> => {
      const res = await api.post<T>(path, body);
      await refresh();
      return res;
    },
    [refresh]
  );

  useEffect(() => {
    let cancelled = false;
    (async () => {
      setLoading(true);
      setError(null);
      try {
        await api.signIn();
        if (cancelled) return;
        await refresh();
      } catch (err) {
        if (cancelled) return;
        setError(
          err instanceof ApiError
            ? `${err.message} (${err.code})`
            : 'Cannot reach the server. Is it running, and is EXPO_PUBLIC_API_URL correct?'
        );
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [refresh, nonce]);

  const value = useMemo<Ctx>(
    () => ({ state, loading, error, refresh, act, retry: () => setNonce((n) => n + 1) }),
    [state, loading, error, refresh, act]
  );

  return <GameContext.Provider value={value}>{children}</GameContext.Provider>;
}

export function useGame(): Ctx {
  const ctx = useContext(GameContext);
  if (!ctx) throw new Error('useGame must be used inside <GameProvider>');
  return ctx;
}

/** Narrowed hook for screens that only render once state exists. */
export function useLoadedGame() {
  const ctx = useGame();
  if (!ctx.state) throw new Error('useLoadedGame used before state loaded');
  return { ...ctx, state: ctx.state };
}
