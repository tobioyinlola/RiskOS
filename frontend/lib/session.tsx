"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";

import {
  api,
  ApiError,
  setCsrfToken,
  setEnvironment as setClientEnvironment,
  unwrap,
  type Environment,
  type Schemas,
} from "@/lib/api/client";

export type ConsoleSession = Schemas["ConsoleSession"];

const ENV_KEY = "riskos.environment";

function storedEnvironment(): Environment {
  try {
    return window.localStorage.getItem(ENV_KEY) === "production"
      ? "production"
      : "test";
  } catch {
    return "test";
  }
}

export async function fetchSession(): Promise<ConsoleSession | null> {
  try {
    const s = unwrap(await api().GET("/api/v1/auth/session"));
    setCsrfToken(s.csrf_token);
    return s;
  } catch (e) {
    if (e instanceof ApiError && (e.status === 401 || e.status === 403))
      return null;
    throw e;
  }
}

interface SessionContextValue {
  session: ConsoleSession;
  environment: Environment;
  setEnvironment: (env: Environment) => void;
  can: (permission: Schemas["Scope"]) => boolean;
  refresh: () => Promise<void>;
}

const SessionContext = createContext<SessionContextValue | null>(null);

export function useSession(): SessionContextValue {
  const ctx = useContext(SessionContext);
  if (!ctx) throw new Error("useSession outside SessionProvider");
  return ctx;
}

export const SESSION_KEY = ["session"] as const;

/**
 * Loads the console session. Without an active session it renders `fallback` (the console
 * layout redirects to sign-in).
 */
export function SessionProvider({
  children,
  onSignedOut,
  fallback,
}: {
  children: ReactNode;
  onSignedOut: () => void;
  fallback: ReactNode;
}) {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: SESSION_KEY,
    queryFn: fetchSession,
    staleTime: 60_000,
    retry: false,
  });
  // The session loads in the browser only, so the stored environment can be read at once.
  const [environment, setEnv] = useState<Environment>(() =>
    typeof window === "undefined" ? "test" : storedEnvironment(),
  );
  useEffect(() => setClientEnvironment(environment), [environment]);

  const signedOut =
    q.isSuccess && (q.data === null || q.data.state !== "active");
  const notified = useRef(false);
  useEffect(() => {
    if (signedOut && !notified.current) {
      notified.current = true;
      onSignedOut();
    }
  }, [signedOut, onSignedOut]);

  const setEnvironment = useCallback(
    (env: Environment) => {
      setClientEnvironment(env);
      setEnv(env);
      try {
        window.localStorage.setItem(ENV_KEY, env);
      } catch {
        // Per-browser convenience only.
      }
      // Every reading belongs to one environment: drop them all.
      qc.removeQueries({
        predicate: (query) => query.queryKey[0] !== SESSION_KEY[0],
      });
    },
    [qc],
  );

  const value = useMemo<SessionContextValue | null>(() => {
    if (!q.data || q.data.state !== "active") return null;
    const perms = new Set(q.data.permissions);
    return {
      session: q.data,
      environment,
      setEnvironment,
      can: (p) => perms.has(p),
      refresh: async () => {
        await qc.invalidateQueries({ queryKey: SESSION_KEY });
      },
    };
  }, [q.data, environment, setEnvironment, qc]);

  if (!value) return <>{fallback}</>;
  return (
    <SessionContext.Provider value={value}>{children}</SessionContext.Provider>
  );
}
