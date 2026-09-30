"use client";

import QRCode from "qrcode";
import { useEffect, useState, type FormEvent } from "react";

import { Button } from "@/components/instrument/button";
import { Field, Input } from "@/components/instrument/field";
import { CodeBlock } from "@/components/instrument/misc";
import { Notice } from "@/components/instrument/states";
import { api, ApiError, unwrap, type Schemas } from "@/lib/api/client";
import {
  assertPasskey,
  createPasskey,
  passkeysSupported,
} from "@/lib/webauthn";

export type AuthState = Schemas["AuthState"];

function message(e: unknown): string {
  if (e instanceof ApiError) return e.message;
  if (e instanceof DOMException && e.name === "NotAllowedError") {
    return "The passkey prompt was dismissed or timed out. Try again.";
  }
  return e instanceof Error ? e.message : "Something went wrong. Try again.";
}

/** Second step of sign-in: a passkey, an authenticator code, or a recovery code. */
export function SecondFactor({
  state,
  onDone,
}: {
  state: AuthState;
  onDone: () => void;
}) {
  const methods = state.mfa_methods ?? [];
  const canPasskey = methods.includes("passkey") && passkeysSupported();
  const [mode, setMode] = useState<"totp" | "recovery">(
    methods.includes("totp") ? "totp" : "recovery",
  );
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);

  const attempt = async (fn: () => Promise<unknown>) => {
    setBusy(true);
    setError(undefined);
    try {
      await fn();
      onDone();
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  };

  const passkey = () =>
    attempt(async () => {
      const options = unwrap(
        await api().POST("/api/v1/auth/mfa/passkeys/login/options"),
      );
      const credential = await assertPasskey(options);
      unwrap(
        await api().POST("/api/v1/auth/mfa/passkeys/login", {
          body: { credential },
        }),
      );
    });

  const onSubmit = (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const code = String(new FormData(e.currentTarget).get("code") ?? "");
    void attempt(async () => {
      const path =
        mode === "totp"
          ? "/api/v1/auth/mfa/totp/verify"
          : "/api/v1/auth/mfa/recovery/verify";
      unwrap(await api().POST(path, { body: { code } }));
    });
  };

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-title">Verify it&apos;s you</h1>
        <p className="mt-1 text-body text-ink-2">
          Your password is correct. Now confirm with your second factor.
        </p>
      </div>
      {error ? <Notice tone="error">{error}</Notice> : null}
      {canPasskey ? (
        <Button
          variant="primary"
          onClick={() => void passkey()}
          disabled={busy}
        >
          Use your passkey
        </Button>
      ) : null}
      {methods.includes("totp") || mode === "recovery" ? (
        <form onSubmit={onSubmit} className="flex flex-col gap-3">
          <Field
            label={mode === "totp" ? "Authenticator code" : "Recovery code"}
          >
            {(p) => (
              <Input
                {...p}
                name="code"
                required
                autoComplete="one-time-code"
                inputMode={mode === "totp" ? "numeric" : "text"}
                autoFocus={!canPasskey}
                placeholder={
                  mode === "totp" ? "6 digits" : "xxxx-xxxx-xxxx-xxxx"
                }
              />
            )}
          </Field>
          <Button
            type="submit"
            variant={canPasskey ? "secondary" : "primary"}
            disabled={busy}
          >
            Verify
          </Button>
        </form>
      ) : null}
      {methods.includes("recovery_code") ? (
        <Button
          variant="ghost"
          size="sm"
          onClick={() => setMode((m) => (m === "totp" ? "recovery" : "totp"))}
        >
          {mode === "totp"
            ? "Lost your device? Use a recovery code"
            : "Use your authenticator app"}
        </Button>
      ) : null}
    </div>
  );
}

/** Recovery codes, shown once, with an explicit confirmation before continuing. */
export function RecoveryCodes({
  codes,
  onContinue,
}: {
  codes: string[];
  onContinue: () => void;
}) {
  const [saved, setSaved] = useState(false);
  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-title">Save your recovery codes</h1>
        <p className="mt-1 text-body text-ink-2">
          Each code signs you in once if you lose your second factor. You will
          not see them again. Store them in a password manager.
        </p>
      </div>
      <CodeBlock value={codes.join("\n")} />
      <div className="flex gap-2">
        <Button
          size="sm"
          onClick={() => void navigator.clipboard.writeText(codes.join("\n"))}
        >
          Copy codes
        </Button>
      </div>
      <label className="flex items-center gap-2 text-body">
        <input
          type="checkbox"
          checked={saved}
          onChange={(e) => setSaved(e.target.checked)}
          className="size-4 accent-[var(--signal-ink)]"
        />
        I have saved these codes somewhere safe
      </label>
      <Button variant="primary" disabled={!saved} onClick={onContinue}>
        Continue to RiskOS
      </Button>
    </div>
  );
}

/**
 * First enrollment of a second factor (mandatory): a passkey (preferred) or an authenticator
 * app. Returns the recovery codes issued with it.
 */
export function Enrollment({
  onEnrolled,
}: {
  onEnrolled: (codes: string[]) => void;
}) {
  const [totp, setTotp] = useState<Schemas["TotpEnrollment"]>();
  const [qr, setQr] = useState<string>();
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);
  const supported = passkeysSupported();

  useEffect(() => {
    if (!totp) return;
    // The library draws in black and white; recolour with theme tokens. The container is pinned
    // to the Paper theme, because scanners expect dark modules on a light ground.
    void QRCode.toString(totp.otpauth_uri, { type: "svg", margin: 1 }).then(
      (svg) =>
        setQr(
          svg
            .replace('fill="#ffffff"', 'class="fill-bg-surface"')
            .replace('stroke="#000000"', 'class="stroke-ink"'),
        ),
    );
  }, [totp]);

  const passkey = async () => {
    setBusy(true);
    setError(undefined);
    try {
      const options = unwrap(
        await api().POST("/api/v1/auth/mfa/passkeys/register/options"),
      );
      const credential = await createPasskey(options);
      const st = unwrap(
        await api().POST("/api/v1/auth/mfa/passkeys/register", {
          body: { name: "Passkey", credential },
        }),
      );
      onEnrolled(st.recovery_codes ?? []);
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  };

  const startTotp = async () => {
    setBusy(true);
    setError(undefined);
    try {
      setTotp(unwrap(await api().POST("/api/v1/auth/mfa/totp/enroll")));
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  };

  const confirm = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const code = String(new FormData(e.currentTarget).get("code") ?? "");
    setBusy(true);
    setError(undefined);
    try {
      const st = unwrap(
        await api().POST("/api/v1/auth/mfa/totp/confirm", { body: { code } }),
      );
      onEnrolled(st.recovery_codes ?? []);
    } catch (err) {
      setError(message(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-title">Add a second factor</h1>
        <p className="mt-1 text-body text-ink-2">
          RiskOS requires one. A passkey (your device&apos;s fingerprint, face
          or PIN) is the strongest and fastest.
        </p>
      </div>
      {error ? <Notice tone="error">{error}</Notice> : null}
      {!totp ? (
        <>
          {supported ? (
            <Button
              variant="primary"
              onClick={() => void passkey()}
              disabled={busy}
            >
              Create a passkey
            </Button>
          ) : (
            <Notice tone="warn">
              This browser does not support passkeys. Use an authenticator app.
            </Notice>
          )}
          <Button onClick={() => void startTotp()} disabled={busy}>
            Use an authenticator app instead
          </Button>
        </>
      ) : (
        <form onSubmit={(e) => void confirm(e)} className="flex flex-col gap-4">
          <p className="text-body text-ink-2">
            Scan this with your authenticator app, or enter the key by hand,
            then type the 6-digit code it shows.
          </p>
          {qr ? (
            <div
              data-theme="light"
              className="size-40 self-start bg-bg-surface"
              role="img"
              aria-label="QR code for your authenticator app"
              dangerouslySetInnerHTML={{ __html: qr }}
            />
          ) : null}
          <div>
            <p className="legend">Setup key</p>
            <p className="mono text-data break-all">{totp.secret}</p>
          </div>
          <Field label="Code from the app">
            {(p) => (
              <Input
                {...p}
                name="code"
                required
                inputMode="numeric"
                autoComplete="one-time-code"
                autoFocus
              />
            )}
          </Field>
          <Button type="submit" variant="primary" disabled={busy}>
            Confirm
          </Button>
        </form>
      )}
    </div>
  );
}
