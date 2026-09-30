"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useCallback, useState } from "react";

import { useReauth } from "@/components/shell/reauth";
import { ApiError } from "@/lib/api/client";

/**
 * Runs a console action: tracks busy and error state, asks for re-authentication when the API
 * requires it, and refreshes the affected readings afterwards.
 */
export function useAction<A extends unknown[], R>(
  fn: (...args: A) => Promise<R>,
  options: { invalidate?: string[]; onSuccess?: (result: R) => void } = {},
) {
  const qc = useQueryClient();
  const reauth = useReauth();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const { invalidate, onSuccess } = options;

  const run = useCallback(
    async (...args: A): Promise<R | undefined> => {
      setBusy(true);
      setError(undefined);
      try {
        const result = await reauth(() => fn(...args));
        if (invalidate?.length) {
          await qc.invalidateQueries({
            predicate: (q) =>
              q.queryKey.some(
                (k) => typeof k === "string" && invalidate.includes(k),
              ),
          });
        }
        onSuccess?.(result);
        return result;
      } catch (e) {
        if (e instanceof ApiError && e.code === "REAUTH_CANCELLED")
          return undefined;
        setError(
          e instanceof ApiError ? e.message : "The action failed. Try again.",
        );
        return undefined;
      } finally {
        setBusy(false);
      }
    },
    [fn, invalidate, onSuccess, qc, reauth],
  );

  return { run, busy, error, clearError: () => setError(undefined) };
}
