"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import {
  useEffect,
  useRef,
  useState,
  useSyncExternalStore,
  type FormEvent,
} from "react";

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
import { Key } from "@/components/ui/icons";

function message(err: unknown, fallback: string): string {
  if (err instanceof ApiError) return err.message;
  if (err instanceof DOMException && err.name === "NotAllowedError")
    return "The passkey prompt was cancelled or timed out. Try again.";
  return fallback;
}

/** Staff sign-in: a passkey, after the identity provider when the deployment has one. */
export function StaffLogin() {
  const router = useRouter();
  const qc = useQueryClient();
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);
  const methods = useQuery({
    queryKey: ["staff-auth-methods"],
    queryFn: async () =>
      unwrap(await adminApi().GET("/api/admin/v1/auth/methods")),
    retry: false,
  });
  const sso = methods.data?.sso_enabled ?? false;
  const ssoRequired = methods.data?.sso_required ?? false;

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

  const signInWithProvider = async () => {
    setBusy(true);
    setError(undefined);
    try {
      const s = unwrap(await adminApi().POST("/api/admin/v1/auth/sso/start"));
      window.location.assign(s.redirect_url);
    } catch (err) {
      setError(message(err, "Single sign-on is unavailable. Try again."));
      setBusy(false);
    }
  };

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-heading-1">RiskOS staff</h1>
        <p className="mt-1 text-body text-text-secondary">
          {ssoRequired
            ? "Sign in with your company identity provider, then confirm with the passkey you registered."
            : "Sign in with the passkey you registered. Staff accounts have no password."}
        </p>
      </div>
      {error ? <Notice tone="error">{error}</Notice> : null}
      {passkeysSupported() ? null : (
        <Notice tone="warn">This browser does not support passkeys.</Notice>
      )}
      {sso ? (
        <Button
          variant="primary"
          disabled={busy}
          onClick={() => void signInWithProvider()}
        >
          Continue with single sign-on
        </Button>
      ) : null}
      {ssoRequired ? null : (
        <Button
          variant={sso ? "secondary" : "primary"}
          icon={<Key />}
          disabled={busy || methods.isPending}
          onClick={() => void signIn()}
        >
          Sign in with passkey
        </Button>
      )}
    </div>
  );
}

/**
 * Where the identity provider returns: finishes its step, then asks for the passkey of the
 * staff member it identified. The request is used up on the first try, so a failure starts
 * again from the sign-in page.
 */
export function StaffSSOCallback() {
  const router = useRouter();
  const qc = useQueryClient();
  const params = useSearchParams();
  const [error, setError] = useState<string>();
  const started = useRef(false);

  useEffect(() => {
    if (started.current) return;
    started.current = true;
    const code = params.get("code");
    const state = params.get("state");
    void (async () => {
      if (params.get("error") || !code || !state) {
        setError("The identity provider did not complete the sign-in.");
        return;
      }
      try {
        // Keep the one-time code out of history before anything else happens.
        window.history.replaceState(null, "", "/admin/sso/callback");
        const c = unwrap(
          await adminApi().POST("/api/admin/v1/auth/sso/callback", {
            body: { code, state },
          }),
        );
        const credential = await assertPasskey(c.options);
        const sess = unwrap(
          await adminApi().POST("/api/admin/v1/auth/sso/login", {
            body: { ceremony_id: c.ceremony_id, credential },
          }),
        );
        setStaffCsrfToken(sess.csrf_token);
        await qc.resetQueries({ queryKey: ["staff"] });
        router.replace("/admin");
      } catch (err) {
        setError(message(err, "Sign-in is unavailable. Try again."));
      }
    })();
  }, [params, qc, router]);

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-heading-1">Signing you in</h1>
      {error ? (
        <>
          <Notice tone="error">{error}</Notice>
          <Button asChild variant="secondary">
            <Link href="/admin/login">Back to sign-in</Link>
          </Button>
        </>
      ) : (
        <p className="text-body text-text-secondary">
          Confirm with your passkey when your browser asks.
        </p>
      )}
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
  const [registered, setRegistered] = useState(false);

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
      if (err instanceof ApiError && err.code === "SSO_REQUIRED") {
        // The passkey is registered; this deployment signs in through the provider first.
        window.history.replaceState(null, "", "/admin/enroll");
        setRegistered(true);
        return;
      }
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

  if (registered) {
    return (
      <div className="flex flex-col gap-4">
        <Notice tone="ok">
          Your passkey is registered. Sign in with single sign-on, then confirm
          with this passkey.
        </Notice>
        <Button asChild variant="primary">
          <Link href="/admin/login">Go to sign-in</Link>
        </Button>
      </div>
    );
  }
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
        icon={<Key />}
        disabled={busy || token === null}
      >
        Register passkey
      </Button>
    </form>
  );
}
