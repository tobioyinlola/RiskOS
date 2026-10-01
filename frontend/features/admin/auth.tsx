"use client";

import { useQueryClient } from "@tanstack/react-query";
import { KeyRound } from "lucide-react";
import { useRouter } from "next/navigation";
import { useState, useSyncExternalStore, type FormEvent } from "react";

import { Button } from "@/components/ui/button";
import { Field, Input } from "@/components/ui/field";
import { Notice } from "@/components/ui/states";
import { adminApi, setStaffCsrfToken } from "@/lib/api/admin-client";
import { ApiError, unwrap } from "@/lib/api/client";
import {
  assertPasskey,
  createPasskey,
  passkeysSupported,
} from "@/lib/webauthn";

function message(err: unknown, fallback: string): string {
  if (err instanceof ApiError) return err.message;
  if (err instanceof DOMException && err.name === "NotAllowedError")
    return "The passkey prompt was cancelled or timed out. Try again.";
  return fallback;
}

/** Staff sign-in: passkeys only, no email or password. */
export function StaffLogin() {
  const router = useRouter();
  const qc = useQueryClient();
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);

  const signIn = async () => {
    setBusy(true);
    setError(undefined);
    try {
      const c = unwrap(
        await adminApi().POST("/api/admin/v1/auth/login/options"),
      );
      const credential = await assertPasskey(c.options);
      const sess = unwrap(
        await adminApi().POST("/api/admin/v1/auth/login", {
          body: { ceremony_id: c.ceremony_id, credential },
        }),
      );
      setStaffCsrfToken(sess.csrf_token);
      await qc.resetQueries({ queryKey: ["staff"] });
      router.replace("/admin");
    } catch (err) {
      setError(message(err, "Sign-in is unavailable. Try again."));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-heading-1">RiskOS staff</h1>
        <p className="mt-1 text-body text-text-secondary">
          Sign in with the passkey you registered. Staff accounts have no
          password.
        </p>
      </div>
      {error ? <Notice tone="error">{error}</Notice> : null}
      {passkeysSupported() ? null : (
        <Notice tone="warn">This browser does not support passkeys.</Notice>
      )}
      <Button
        variant="primary"
        icon={<KeyRound />}
        disabled={busy}
        onClick={() => void signIn()}
      >
        Sign in with passkey
      </Button>
    </div>
  );
}

function subscribeHash(onChange: () => void) {
  window.addEventListener("hashchange", onChange);
  return () => window.removeEventListener("hashchange", onChange);
}

/** Registers a staff member's passkey from the link the operator CLI printed. */
export function StaffEnroll() {
  const router = useRouter();
  const qc = useQueryClient();
  // The token is in the URL fragment, which only the browser sees.
  const token = useSyncExternalStore(
    subscribeHash,
    () => new URLSearchParams(window.location.hash.slice(1)).get("token") ?? "",
    () => null,
  );
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);

  const onSubmit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const name = String(new FormData(e.currentTarget).get("name") ?? "");
    setBusy(true);
    setError(undefined);
    try {
      const c = unwrap(
        await adminApi().POST("/api/admin/v1/auth/enroll/options", {
          body: { token: token ?? "" },
        }),
      );
      const credential = await createPasskey(c.options);
      const sess = unwrap(
        await adminApi().POST("/api/admin/v1/auth/enroll", {
          body: {
            token: token ?? "",
            ceremony_id: c.ceremony_id,
            name: name || undefined,
            credential,
          },
        }),
      );
      setStaffCsrfToken(sess.csrf_token);
      // Keep the used token out of history.
      window.history.replaceState(null, "", "/admin/enroll");
      await qc.resetQueries({ queryKey: ["staff"] });
      router.replace("/admin");
    } catch (err) {
      setError(
        message(
          err,
          "Registration failed. Ask the operator for a new link if it has expired.",
        ),
      );
    } finally {
      setBusy(false);
    }
  };

  if (token === "") {
    return (
      <Notice tone="error">
        This link has no enrollment token. Ask the operator for a new link.
      </Notice>
    );
  }
  return (
    <form onSubmit={(e) => void onSubmit(e)} className="flex flex-col gap-4">
      <div>
        <h1 className="text-heading-1">Register your staff passkey</h1>
        <p className="mt-1 text-body text-text-secondary">
          Staff sign in with a passkey only. This link works once.
        </p>
      </div>
      {error ? <Notice tone="error">{error}</Notice> : null}
      <Field
        label="Passkey name"
        hint="To recognise it later, e.g. Work laptop."
      >
        {(p) => <Input {...p} name="name" maxLength={100} />}
      </Field>
      <Button
        type="submit"
        variant="primary"
        icon={<KeyRound />}
        disabled={busy || token === null}
      >
        Register passkey
      </Button>
    </form>
  );
}
