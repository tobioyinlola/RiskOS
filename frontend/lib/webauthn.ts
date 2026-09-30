/**
 * Passkey (WebAuthn) helpers: the API sends and expects the standard JSON encodings
 * (base64url for binary fields); the browser API takes and returns ArrayBuffers.
 */

export function b64urlToBuffer(s: string): ArrayBuffer {
  const pad = "=".repeat((4 - (s.length % 4)) % 4);
  const bin = atob((s + pad).replaceAll("-", "+").replaceAll("_", "/"));
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out.buffer;
}

export function bufferToB64url(buf: ArrayBuffer): string {
  const bytes = new Uint8Array(buf);
  let bin = "";
  for (const b of bytes) bin += String.fromCharCode(b);
  return btoa(bin).replaceAll("+", "-").replaceAll("/", "_").replace(/=+$/, "");
}

type Json = Record<string, unknown>;

function descriptors(
  list: unknown,
): PublicKeyCredentialDescriptor[] | undefined {
  if (!Array.isArray(list)) return undefined;
  return list.map((d: Json) => ({
    ...(d as object),
    type: "public-key",
    id: b64urlToBuffer(String(d.id)),
  })) as PublicKeyCredentialDescriptor[];
}

export function passkeysSupported(): boolean {
  return (
    typeof window !== "undefined" &&
    "PublicKeyCredential" in window &&
    !!navigator.credentials
  );
}

/** Creates a passkey from the API's creation options and returns the response to send back. */
export async function createPasskey(options: Json): Promise<Json> {
  const pk = (options.publicKey ?? options) as Json;
  const user = pk.user as Json;
  const publicKey = {
    ...pk,
    challenge: b64urlToBuffer(String(pk.challenge)),
    user: { ...user, id: b64urlToBuffer(String(user.id)) },
    excludeCredentials: descriptors(pk.excludeCredentials),
  } as unknown as PublicKeyCredentialCreationOptions;
  const cred = (await navigator.credentials.create({
    publicKey,
  })) as PublicKeyCredential | null;
  if (!cred) throw new Error("No passkey was created.");
  const res = cred.response as AuthenticatorAttestationResponse;
  return {
    id: cred.id,
    rawId: bufferToB64url(cred.rawId),
    type: cred.type,
    authenticatorAttachment: cred.authenticatorAttachment ?? undefined,
    clientExtensionResults: cred.getClientExtensionResults(),
    response: {
      clientDataJSON: bufferToB64url(res.clientDataJSON),
      attestationObject: bufferToB64url(res.attestationObject),
      transports:
        typeof res.getTransports === "function" ? res.getTransports() : [],
    },
  };
}

/** Uses a passkey with the API's request options and returns the assertion to send back. */
export async function assertPasskey(options: Json): Promise<Json> {
  const pk = (options.publicKey ?? options) as Json;
  const publicKey = {
    ...pk,
    challenge: b64urlToBuffer(String(pk.challenge)),
    allowCredentials: descriptors(pk.allowCredentials),
  } as unknown as PublicKeyCredentialRequestOptions;
  const cred = (await navigator.credentials.get({
    publicKey,
  })) as PublicKeyCredential | null;
  if (!cred) throw new Error("No passkey was used.");
  const res = cred.response as AuthenticatorAssertionResponse;
  return {
    id: cred.id,
    rawId: bufferToB64url(cred.rawId),
    type: cred.type,
    authenticatorAttachment: cred.authenticatorAttachment ?? undefined,
    clientExtensionResults: cred.getClientExtensionResults(),
    response: {
      clientDataJSON: bufferToB64url(res.clientDataJSON),
      authenticatorData: bufferToB64url(res.authenticatorData),
      signature: bufferToB64url(res.signature),
      userHandle: res.userHandle ? bufferToB64url(res.userHandle) : undefined,
    },
  };
}
