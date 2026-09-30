"use client";

import { useQueryClient } from "@tanstack/react-query";
import {
  createContext,
  useCallback,
  useContext,
  useRef,
  useState,
  type FormEvent,
  type ReactNode,
} from "react";

import { Button } from "@/components/instrument/button";
import { Field, Input } from "@/components/instrument/field";
import { Modal } from "@/components/instrument/overlay";
import { Notice } from "@/components/instrument/states";
import { api, ApiError, unwrap } from "@/lib/api/client";
import { SESSION_KEY, useSession } from "@/lib/session";
import { assertPasskey, passkeysSupported } from "@/lib/webauthn";

type Run = <T>(action: () => Promise<T>) => Promise<T>;

const ReauthContext = createContext<Run | null>(null);

/**
 * Wraps a high-impact action: if the API answers REAUTH_REQUIRED, asks the user to verify it's
 * them again (passkey, authenticator code or recovery code), then retries the action once.
 */
export function useReauth(): Run {
  const ctx = useContext(ReauthContext);
  if (!ctx) throw new Error("useReauth outside ReauthProvider");
  return ctx;
}

export function ReauthProvider({ children }: { children: ReactNode }) {
  const { session } = useSession();
  const qc = useQueryClient();
  const [open, setOpen] = useState(false);
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);
  const [recovery, setRecovery] = useState(false);
  const pending = useRef<{
    resolve: () => void;
    reject: (e: unknown) => void;
  } | null>(null);

  const ask = useCallback(
    () =>
      new Promise<void>((resolve, reject) => {
        pending.current = { resolve, reject };
        setError(undefined);
        setRecovery(false);
        setOpen(true);
      }),
    [],
  );

  const run: Run = useCallback(
    async (action) => {
      try {
        return await action();
      } catch (e) {
        if (!(e instanceof ApiError && e.code === "REAUTH_REQUIRED")) throw e;
        await ask();
        return action();
      }
    },
    [ask],
  );

  const done = async () => {
    await qc.invalidateQueries({ queryKey: SESSION_KEY });
    setOpen(false);
    pending.current?.resolve();
    pending.current = null;
  };

  const verify = async (fn: () => Promise<unknown>) => {
    setBusy(true);
    setError(undefined);
    try {
      await fn();
      await done();
    } catch (e) {
      setError(
        e instanceof ApiError
          ? e.message
          : "The verification failed. Try again.",
      );
    } finally {
      setBusy(false);
    }
  };

  const onCode = (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const code = String(new FormData(e.currentTarget).get("code") ?? "");
    void verify(async () =>
      recovery
        ? unwrap(
            await api().POST("/api/v1/auth/mfa/recovery/verify", {
              body: { code },
            }),
          )
        : unwrap(
            await api().POST("/api/v1/auth/mfa/totp/verify", {
              body: { code },
            }),
          ),
    );
  };

  const onPasskey = () =>
    verify(async () => {
      const options = unwrap(
        await api().POST("/api/v1/auth/mfa/passkeys/login/options"),
      );
      const credential = await assertPasskey(options);
      return unwrap(
        await api().POST("/api/v1/auth/mfa/passkeys/login", {
          body: { credential },
        }),
      );
    });

  const cancel = () => {
    setOpen(false);
    pending.current?.reject(
      new ApiError(403, "REAUTH_CANCELLED", "Verification was cancelled."),
    );
    pending.current = null;
  };

  const hasPasskey = session.mfa.passkeys.length > 0 && passkeysSupported();
  return (
    <ReauthContext.Provider value={run}>
      {children}
      <Modal
        open={open}
        onOpenChange={(o) => (o ? setOpen(true) : cancel())}
        title="Verify it's you"
        description="This action changes how RiskOS or your team works. Confirm with your second factor; it stays confirmed for 10 minutes."
      >
        <div className="flex flex-col gap-4">
          {error ? <Notice tone="error">{error}</Notice> : null}
          {hasPasskey && !recovery ? (
            <Button
              variant="primary"
              onClick={() => void onPasskey()}
              disabled={busy}
            >
              Use your passkey
            </Button>
          ) : null}
          {session.mfa.totp || recovery ? (
            <form onSubmit={onCode} className="flex flex-col gap-3">
              <Field label={recovery ? "Recovery code" : "Authenticator code"}>
                {(p) => (
                  <Input
                    {...p}
                    name="code"
                    autoComplete="one-time-code"
                    inputMode={recovery ? "text" : "numeric"}
                    required
                    autoFocus={!hasPasskey}
                  />
                )}
              </Field>
              <Button
                type="submit"
                variant={hasPasskey ? "secondary" : "primary"}
                disabled={busy}
              >
                Verify
              </Button>
            </form>
          ) : null}
          <div className="flex justify-between">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setRecovery((r) => !r)}
            >
              {recovery ? "Use your second factor" : "Use a recovery code"}
            </Button>
            <Button variant="ghost" size="sm" onClick={cancel}>
              Cancel
            </Button>
          </div>
        </div>
      </Modal>
    </ReauthContext.Provider>
  );
}
