"use client";

import { useQueryClient } from "@tanstack/react-query";
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
import { api, ApiError, unwrap } from "@/lib/api/client";

import { Enrollment, RecoveryCodes, SecondFactor, type AuthState } from "./mfa";

/** Only same-site relative paths are followed after sign-in (no open redirects). */
function safeNext(next: string | null): string {
  return next && next.startsWith("/") && !next.startsWith("//") ? next : "/";
}

export function Login() {
  const router = useRouter();
  const params = useSearchParams();
  const qc = useQueryClient();
  const [state, setState] = useState<AuthState>();
  const [codes, setCodes] = useState<string[]>();
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);
  const [sso, setSSO] = useState(false);

  const enter = async () => {
    await qc.resetQueries();
    router.replace(safeNext(params.get("next")) as "/");
  };

  const startSSO = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    setBusy(true);
    setError(undefined);
    try {
      const st = unwrap(
        await api().POST("/api/v1/auth/sso/start", {
          body: { email: String(form.get("email") ?? "") },
        }),
      );
      // Leaves the console for the identity provider, which returns to /sso/callback.
      window.location.assign(st.redirect_url);
    } catch (err) {
      setError(
        err instanceof ApiError
          ? err.message
          : "Single sign-on is unavailable. Try again.",
      );
      setBusy(false);
    }
  };

  const onSubmit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    setBusy(true);
    setError(undefined);
    try {
      const st = unwrap(
        await api().POST("/api/v1/auth/login", {
          body: {
            email: String(form.get("email") ?? ""),
            password: String(form.get("password") ?? ""),
          },
        }),
      );
      if (st.state === "active") await enter();
      else setState(st);
    } catch (err) {
      setError(
        err instanceof ApiError
          ? err.message
          : "Sign-in is unavailable. Try again.",
      );
    } finally {
      setBusy(false);
    }
  };

  if (codes)
    return <RecoveryCodes codes={codes} onContinue={() => void enter()} />;
  if (state?.state === "mfa_required")
    return <SecondFactor state={state} onDone={() => void enter()} />;
  if (state?.state === "enrollment_required") {
    return (
      <Enrollment onEnrolled={(c) => (c.length ? setCodes(c) : void enter())} />
    );
  }

  if (sso) {
    return (
      <form onSubmit={(e) => void startSSO(e)} className="flex flex-col gap-4">
        <div>
          <h1 className="text-heading-1">Sign in with single sign-on</h1>
          <p className="mt-1 text-body text-text-secondary">
            We&apos;ll send you to your organization&apos;s identity provider.
          </p>
        </div>
        {error ? <Notice tone="error">{error}</Notice> : null}
        <Field label="Work email">
          {(p) => (
            <Input
              {...p}
              name="email"
              type="email"
              autoComplete="username"
              required
              autoFocus
            />
          )}
        </Field>
        <Button type="submit" variant="primary" disabled={busy}>
          Continue
        </Button>
        <Button
          variant="ghost"
          onClick={() => {
            setSSO(false);
            setError(undefined);
          }}
        >
          Sign in with a password instead
        </Button>
      </form>
    );
  }

  return (
    <form onSubmit={(e) => void onSubmit(e)} className="flex flex-col gap-4">
      <div>
        <h1 className="text-heading-1">Sign in</h1>
        <p className="mt-1 text-body text-text-secondary">
          Use the email your administrator invited.
        </p>
      </div>
      {error ? <Notice tone="error">{error}</Notice> : null}
      <Field label="Email">
        {(p) => (
          <Input
            {...p}
            name="email"
            type="email"
            autoComplete="username"
            required
            autoFocus
          />
        )}
      </Field>
      <Field label="Password">
        {(p) => (
          <Input
            {...p}
            name="password"
            type="password"
            autoComplete="current-password"
            required
          />
        )}
      </Field>
      <Button type="submit" variant="primary" disabled={busy}>
        Continue
      </Button>
      <Button
        variant="secondary"
        onClick={() => {
          setSSO(true);
          setError(undefined);
        }}
      >
        Sign in with single sign-on
      </Button>
      <p className="text-body-sm text-text-muted">
        No account yet? Ask your RiskOS administrator for an invite link.
      </p>
    </form>
  );
}

/** Where the identity provider sends the browser back: finishes single sign-on once. */
export function SSOCallback() {
  const router = useRouter();
  const params = useSearchParams();
  const qc = useQueryClient();
  const [state, setState] = useState<AuthState>();
  const [codes, setCodes] = useState<string[]>();
  const [error, setError] = useState<string>();
  // Read once: the address bar is cleaned up below, which changes the search parameters.
  const [returned] = useState(() => ({
    code: params.get("code"),
    state: params.get("state"),
    error: params.get("error"),
  }));
  const incomplete =
    returned.error || !returned.code || !returned.state
      ? returned.error === "access_denied"
        ? "Sign-in was cancelled at your identity provider."
        : "Your identity provider did not complete the sign-in."
      : undefined;
  // The state is single use: never send it twice (effects can run twice in development).
  const sent = useRef(false);

  const enter = async () => {
    await qc.resetQueries();
    router.replace("/");
  };

  useEffect(() => {
    if (sent.current || incomplete || !returned.code || !returned.state) return;
    sent.current = true;
    const { code, state: st } = returned;
    // Keep the code out of the address bar and history.
    window.history.replaceState(null, "", "/sso/callback");
    void (async () => {
      try {
        const next = unwrap(
          await api().POST("/api/v1/auth/sso/callback", {
            body: { code, state: st },
          }),
        );
        if (next.state === "active") await enter();
        else setState(next);
      } catch (err) {
        setError(
          err instanceof ApiError
            ? err.message
            : "Single sign-on is unavailable. Try again.",
        );
      }
    })();
    // Runs once per visit; enter only navigates.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  if (codes)
    return <RecoveryCodes codes={codes} onContinue={() => void enter()} />;
  if (state?.state === "mfa_required")
    return <SecondFactor state={state} onDone={() => void enter()} />;
  if (state?.state === "enrollment_required") {
    return (
      <Enrollment onEnrolled={(c) => (c.length ? setCodes(c) : void enter())} />
    );
  }
  if (error || incomplete) {
    return (
      <div className="flex flex-col gap-4">
        <h1 className="text-heading-1">Sign-in did not complete</h1>
        <Notice tone="error">{error ?? incomplete}</Notice>
        <Button variant="primary" onClick={() => router.replace("/login")}>
          Back to sign-in
        </Button>
      </div>
    );
  }
  return (
    <p className="text-body text-text-secondary" role="status">
      Finishing sign-in…
    </p>
  );
}

function subscribeHash(onChange: () => void) {
  window.addEventListener("hashchange", onChange);
  return () => window.removeEventListener("hashchange", onChange);
}

export function AcceptInvite() {
  const router = useRouter();
  const qc = useQueryClient();
  // The token is in the URL fragment, which only the browser sees (null while rendering on the
  // server and hydrating).
  const token = useSyncExternalStore(
    subscribeHash,
    () => new URLSearchParams(window.location.hash.slice(1)).get("token") ?? "",
    () => null,
  );
  const [step, setStep] = useState<"form" | "enroll" | "codes">("form");
  const [codes, setCodes] = useState<string[]>([]);
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);

  const enter = async () => {
    await qc.resetQueries();
    router.replace("/");
  };

  const onSubmit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    if (form.get("password") !== form.get("confirm")) {
      setError("The passwords do not match.");
      return;
    }
    setBusy(true);
    setError(undefined);
    try {
      unwrap(
        await api().POST("/api/v1/auth/invites/accept", {
          body: {
            token: token ?? "",
            name: String(form.get("name") ?? ""),
            password: String(form.get("password") ?? ""),
          },
        }),
      );
      // The token has done its job; keep it out of the history.
      window.history.replaceState(null, "", "/accept-invite");
      setStep("enroll");
    } catch (err) {
      setError(
        err instanceof ApiError
          ? err.message
          : "The invite could not be accepted. Try again.",
      );
    } finally {
      setBusy(false);
    }
  };

  if (step === "codes")
    return <RecoveryCodes codes={codes} onContinue={() => void enter()} />;
  if (step === "enroll") {
    return (
      <Enrollment
        onEnrolled={(c) => {
          setCodes(c);
          if (c.length) setStep("codes");
          else void enter();
        }}
      />
    );
  }
  if (token === null) return null;
  if (!token) {
    return (
      <div className="flex flex-col gap-2">
        <h1 className="text-heading-1">Invite link incomplete</h1>
        <p className="text-body text-text-secondary">
          Open the full link your administrator shared, including everything
          after the #.
        </p>
      </div>
    );
  }
  return (
    <form onSubmit={(e) => void onSubmit(e)} className="flex flex-col gap-4">
      <div>
        <h1 className="text-heading-1">Set up your account</h1>
        <p className="mt-1 text-body text-text-secondary">
          Choose a name and a password. You will add a second factor next.
        </p>
      </div>
      {error ? <Notice tone="error">{error}</Notice> : null}
      <Field label="Your name">
        {(p) => (
          <Input
            {...p}
            name="name"
            autoComplete="name"
            required
            maxLength={100}
            autoFocus
          />
        )}
      </Field>
      <Field
        label="Password"
        hint="At least 12 characters. A passphrase of a few words works well."
      >
        {(p) => (
          <Input
            {...p}
            name="password"
            type="password"
            autoComplete="new-password"
            required
            minLength={12}
            maxLength={128}
          />
        )}
      </Field>
      <Field label="Confirm password">
        {(p) => (
          <Input
            {...p}
            name="confirm"
            type="password"
            autoComplete="new-password"
            required
            minLength={12}
            maxLength={128}
          />
        )}
      </Field>
      <Button type="submit" variant="primary" disabled={busy}>
        Create account
      </Button>
    </form>
  );
}
