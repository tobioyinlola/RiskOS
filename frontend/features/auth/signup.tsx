"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import {
  useEffect,
  useRef,
  useState,
  type FormEvent,
  type ReactNode,
} from "react";

import { Button } from "@/components/ui/button";
import { Field, Input } from "@/components/ui/field";
import { Google, Mail } from "@/components/ui/icons";
import { Notice } from "@/components/ui/states";
import { api, ApiError, unwrap } from "@/lib/api/client";
import { cn } from "@/lib/cn";

import { Enrollment, RecoveryCodes, SecondFactor, type AuthState } from "./mfa";

/** Which ways to sign up the server offers (email needs email delivery; Google its client). */
export function useSignupOptions() {
  return useQuery({
    queryKey: ["signup-options"],
    queryFn: async () => unwrap(await api().GET("/api/v1/auth/signup/options")),
    staleTime: 5 * 60_000,
  });
}

/** The steps of creating a workspace, shown above each one. */
const STEPS = [
  "Your details",
  "Verify email",
  "Secure account",
  "Compliance",
] as const;

export function SignupSteps({ current }: { current: number }) {
  return (
    <ol aria-label="Sign-up progress" className="mb-7 flex items-center gap-2">
      {STEPS.map((label, i) => (
        <li
          key={label}
          aria-current={i === current ? "step" : undefined}
          className="flex min-w-0 flex-1 flex-col gap-1.5"
        >
          <span
            aria-hidden="true"
            className={cn(
              "h-1 rounded-full transition-colors duration-200",
              i < current
                ? "bg-ink"
                : i === current
                  ? "bg-brand-blue"
                  : "bg-bg-sunken",
            )}
          />
          <span
            className={cn(
              "truncate text-label",
              i === current ? "text-text-primary" : "text-text-muted",
            )}
          >
            <span className="sr-only">
              {i < current ? "Done: " : i === current ? "Current: " : ""}
            </span>
            {label}
          </span>
        </li>
      ))}
    </ol>
  );
}

function Heading({
  title,
  accent,
  children,
}: {
  title: string;
  accent?: string;
  children?: ReactNode;
}) {
  return (
    <div className="mb-6">
      <h1 className="text-heading-1">
        {title}{" "}
        {accent ? <span className="accent-italic">{accent}</span> : null}
      </h1>
      {children ? (
        <p className="mt-2 text-body text-text-secondary">{children}</p>
      ) : null}
    </div>
  );
}

/** "Continue with Google": leaves for Google, which returns to /signup/google. */
export function GoogleButton({
  label = "Continue with Google",
}: {
  label?: string;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const start = async () => {
    setBusy(true);
    setError(undefined);
    try {
      const st = unwrap(await api().POST("/api/v1/auth/google/start"));
      window.location.assign(st.redirect_url);
    } catch (err) {
      setError(
        err instanceof ApiError
          ? err.message
          : "Google sign-in is unavailable. Try again.",
      );
      setBusy(false);
    }
  };
  return (
    <div className="flex flex-col gap-2">
      <Button
        className="w-full"
        icon={<Google weight="bold" />}
        disabled={busy}
        onClick={() => void start()}
      >
        {label}
      </Button>
      {error ? <Notice tone="error">{error}</Notice> : null}
    </div>
  );
}

export function OrDivider({ children = "or" }: { children?: ReactNode }) {
  return (
    <div
      className="my-5 flex items-center gap-3 text-label text-text-muted"
      role="separator"
    >
      <span className="h-px flex-1 bg-border-subtle" />
      {children}
      <span className="h-px flex-1 bg-border-subtle" />
    </div>
  );
}

/** After the second factor: on to the compliance onboarding. */
function useFinish() {
  const router = useRouter();
  const qc = useQueryClient();
  return async () => {
    await qc.resetQueries();
    router.replace("/onboarding");
  };
}

/** Second factor enrollment and recovery codes, the third step of every sign-up. */
function SecureAccount({ state }: { state: AuthState }) {
  const finish = useFinish();
  const [codes, setCodes] = useState<string[]>();
  if (codes) {
    return (
      <>
        <SignupSteps current={2} />
        <RecoveryCodes codes={codes} onContinue={() => void finish()} />
      </>
    );
  }
  if (state.state === "mfa_required") {
    // Signing in with Google to an existing account: its usual second factor.
    return <SecondFactor state={state} onDone={() => void finish()} />;
  }
  return (
    <>
      <SignupSteps current={2} />
      <Enrollment
        onEnrolled={(c) => (c.length ? setCodes(c) : void finish())}
      />
    </>
  );
}

/** /signup: create a workspace with Google, or with email, business name and password. */
export function Signup() {
  const options = useSignupOptions();
  const [pending, setPending] = useState<{
    email: string;
    resendAfter: number;
  }>();
  const [auth, setAuth] = useState<AuthState>();
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);

  if (auth) return <SecureAccount state={auth} />;
  if (pending) {
    return (
      <VerifyEmail
        email={pending.email}
        resendAfter={pending.resendAfter}
        onVerified={setAuth}
        onRestart={() => setPending(undefined)}
      />
    );
  }

  const onSubmit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    setBusy(true);
    setError(undefined);
    try {
      const p = unwrap(
        await api().POST("/api/v1/auth/signup", {
          body: {
            name: String(form.get("name") ?? ""),
            email: String(form.get("email") ?? ""),
            business_name: String(form.get("business_name") ?? ""),
            password: String(form.get("password") ?? ""),
          },
        }),
      );
      setPending({ email: p.email, resendAfter: p.resend_after_seconds });
    } catch (err) {
      setError(
        err instanceof ApiError
          ? err.message
          : "Sign-up is unavailable. Try again.",
      );
    } finally {
      setBusy(false);
    }
  };

  const email = options.data?.email ?? false;
  const google = options.data?.google ?? false;
  return (
    <>
      <SignupSteps current={0} />
      <Heading title="Create your" accent="workspace.">
        Start in the test environment today. Production opens after a short
        compliance review.
      </Heading>
      {options.isLoading ? (
        <p role="status" className="text-body text-text-muted">
          Loading…
        </p>
      ) : null}
      {options.isSuccess && !email && !google ? (
        <Notice tone="info">
          Sign-up is not open on this RiskOS. Ask your administrator for an
          invite.
        </Notice>
      ) : null}
      {google ? <GoogleButton /> : null}
      {google && email ? <OrDivider>or with your work email</OrDivider> : null}
      {email ? (
        <form
          onSubmit={(e) => void onSubmit(e)}
          className="flex flex-col gap-4"
        >
          {error ? <Notice tone="error">{error}</Notice> : null}
          <Field label="Full name">
            {(p) => (
              <Input
                {...p}
                name="name"
                autoComplete="name"
                required
                maxLength={100}
              />
            )}
          </Field>
          <Field label="Work email">
            {(p) => (
              <Input
                {...p}
                name="email"
                type="email"
                autoComplete="email"
                required
                maxLength={254}
              />
            )}
          </Field>
          <Field
            label="Business name"
            hint="Your workspace's name. You can add legal details during onboarding."
          >
            {(p) => (
              <Input
                {...p}
                name="business_name"
                autoComplete="organization"
                required
                minLength={2}
                maxLength={200}
              />
            )}
          </Field>
          <Field
            label="Password"
            hint="At least 12 characters, not your email. A passphrase works well."
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
          <Button type="submit" variant="primary" arrow disabled={busy}>
            Create workspace
          </Button>
          <p className="text-label text-text-muted">
            By continuing you agree to the RiskOS terms and privacy notice. You
            confirm them formally during onboarding.
          </p>
        </form>
      ) : null}
      <p className="mt-6 text-body-sm text-text-secondary">
        Already have an account?{" "}
        <Link href="/login" className="font-medium text-accent">
          Sign in
        </Link>
      </p>
    </>
  );
}

/** Step two: the 6-digit code from the email. */
function VerifyEmail({
  email,
  resendAfter,
  onVerified,
  onRestart,
}: {
  email: string;
  resendAfter: number;
  onVerified: (s: AuthState) => void;
  onRestart: () => void;
}) {
  const [code, setCode] = useState("");
  const [error, setError] = useState<string>();
  const [expired, setExpired] = useState(false);
  const [notice, setNotice] = useState<string>();
  const [busy, setBusy] = useState(false);
  const [wait, setWait] = useState(resendAfter);

  useEffect(() => {
    if (wait <= 0) return;
    const t = setTimeout(() => setWait((w) => w - 1), 1000);
    return () => clearTimeout(t);
  }, [wait]);

  const verify = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    setBusy(true);
    setError(undefined);
    setNotice(undefined);
    try {
      onVerified(
        unwrap(
          await api().POST("/api/v1/auth/signup/verify", { body: { code } }),
        ),
      );
    } catch (err) {
      if (err instanceof ApiError && err.code === "SIGNUP_INVALID")
        setExpired(true);
      setError(
        err instanceof ApiError
          ? err.message
          : "Verification is unavailable. Try again.",
      );
      setCode("");
    } finally {
      setBusy(false);
    }
  };

  const resend = async () => {
    setError(undefined);
    setNotice(undefined);
    try {
      const p = unwrap(await api().POST("/api/v1/auth/signup/resend"));
      setWait(p.resend_after_seconds);
      setNotice("A new code is on its way. The previous one no longer works.");
    } catch (err) {
      if (err instanceof ApiError && err.code === "SIGNUP_INVALID")
        setExpired(true);
      setError(
        err instanceof ApiError
          ? err.message
          : "Could not send a new code. Try again.",
      );
    }
  };

  return (
    <>
      <SignupSteps current={1} />
      <span className="mb-5 inline-flex size-12 items-center justify-center rounded-md bg-blue-50 text-accent [&_svg]:size-6">
        <Mail aria-hidden="true" />
      </span>
      <Heading title="Check your" accent="email.">
        We sent a 6-digit code to{" "}
        <b className="font-semibold text-text-primary">{email}</b>. It works for
        an hour.
      </Heading>
      {expired ? (
        <div className="flex flex-col gap-4">
          <Notice tone="error">{error}</Notice>
          <Button variant="primary" onClick={onRestart}>
            Start again
          </Button>
        </div>
      ) : (
        <form onSubmit={(e) => void verify(e)} className="flex flex-col gap-4">
          {error ? <Notice tone="error">{error}</Notice> : null}
          {notice ? <Notice tone="ok">{notice}</Notice> : null}
          <Field label="Verification code">
            {(p) => (
              <Input
                {...p}
                name="code"
                value={code}
                onChange={(e) =>
                  setCode(e.target.value.replace(/\D/g, "").slice(0, 6))
                }
                inputMode="numeric"
                autoComplete="one-time-code"
                pattern="[0-9]{6}"
                required
                autoFocus
                className="h-14 text-center text-heading-2 tracking-[0.5em]"
              />
            )}
          </Field>
          <Button
            type="submit"
            variant="primary"
            disabled={busy || code.length !== 6}
          >
            Verify and create workspace
          </Button>
          <div className="flex flex-wrap items-center justify-between gap-2 text-body-sm">
            <button
              type="button"
              onClick={() => void resend()}
              disabled={wait > 0}
              className="font-medium text-accent disabled:text-text-muted"
            >
              {wait > 0 ? `Send a new code in ${wait}s` : "Send a new code"}
            </button>
            <button
              type="button"
              onClick={onRestart}
              className="text-text-secondary hover:text-text-primary"
            >
              Use a different email
            </button>
          </div>
        </form>
      )}
    </>
  );
}

/** /signup/google: where Google sends the browser back. */
export function GoogleCallback() {
  const router = useRouter();
  const params = useSearchParams();
  const qc = useQueryClient();
  const [auth, setAuth] = useState<AuthState>();
  const [business, setBusiness] = useState<{ name: string; email: string }>();
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);
  const [returned] = useState(() => ({
    code: params.get("code"),
    state: params.get("state"),
    error: params.get("error"),
  }));
  const incomplete =
    returned.error || !returned.code || !returned.state
      ? returned.error === "access_denied"
        ? "You cancelled at Google."
        : "Google did not complete the sign-in."
      : undefined;
  // The state is single use: never send it twice (effects can run twice in development).
  const sent = useRef(false);

  useEffect(() => {
    if (sent.current || incomplete || !returned.code || !returned.state) return;
    sent.current = true;
    const { code, state } = returned;
    // Keep the code out of the address bar and history.
    window.history.replaceState(null, "", "/signup/google");
    void (async () => {
      try {
        const res = unwrap(
          await api().POST("/api/v1/auth/google/callback", {
            body: { code, state },
          }),
        );
        if (res.outcome === "needs_business") {
          setBusiness({ name: res.name ?? "", email: res.email ?? "" });
        } else if (res.auth?.state === "active") {
          await qc.resetQueries();
          router.replace("/");
        } else if (res.auth) {
          setAuth(res.auth);
        }
      } catch (err) {
        setError(
          err instanceof ApiError
            ? err.message
            : "Google sign-in is unavailable. Try again.",
        );
      }
    })();
    // Runs once per visit.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const complete = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    setBusy(true);
    setError(undefined);
    try {
      setAuth(
        unwrap(
          await api().POST("/api/v1/auth/signup/complete", {
            body: { business_name: String(form.get("business_name") ?? "") },
          }),
        ),
      );
    } catch (err) {
      setError(
        err instanceof ApiError
          ? err.message
          : "Sign-up is unavailable. Try again.",
      );
    } finally {
      setBusy(false);
    }
  };

  if (auth) return <SecureAccount state={auth} />;
  if (business) {
    return (
      <>
        <SignupSteps current={1} />
        <Heading
          title={`Welcome, ${business.name.split(" ")[0] || "there"}.`}
          accent="Name your workspace."
        >
          Google confirmed{" "}
          <b className="font-semibold text-text-primary">{business.email}</b>.
          One more detail and your workspace is ready.
        </Heading>
        <form
          onSubmit={(e) => void complete(e)}
          className="flex flex-col gap-4"
        >
          {error ? <Notice tone="error">{error}</Notice> : null}
          <Field label="Business name">
            {(p) => (
              <Input
                {...p}
                name="business_name"
                autoComplete="organization"
                required
                minLength={2}
                maxLength={200}
                autoFocus
              />
            )}
          </Field>
          <Button type="submit" variant="primary" arrow disabled={busy}>
            Create workspace
          </Button>
        </form>
      </>
    );
  }
  if (error || incomplete) {
    return (
      <div className="flex flex-col gap-4">
        <Heading title="Google sign-in did not" accent="complete." />
        <Notice tone="error">{error ?? incomplete}</Notice>
        <Button variant="primary" onClick={() => router.replace("/signup")}>
          Back to sign-up
        </Button>
      </div>
    );
  }
  return (
    <p className="text-body text-text-secondary" role="status">
      Finishing with Google…
    </p>
  );
}
