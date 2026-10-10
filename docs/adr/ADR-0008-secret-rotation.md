# ADR-0008: Rotating the server-side secrets

- **Status:** proposed
- **Date:** 2026-10-10
- **Deciders:** product owner, engineering

## Context

Four server-side secrets are each a single value read at start-up, so changing one in place
breaks something that cannot be repaired without the old value:

| Secret | What it protects | What an in-place change does |
|---|---|---|
| `RISKOS_API_KEY_PEPPER` | HMAC of every stored API key secret | Every API key stops authenticating; the secrets are not stored and cannot be rehashed |
| `RISKOS_WEBHOOK_SIGNING_KEY` | Derives every webhook endpoint's signing secret | Every customer's stored signing secret stops matching our signatures |
| `RISKOS_CONSOLE_SECRET` | Console session, invite and challenge tokens | Signed-in users and open invites are invalidated |
| `RISKOS_FEATURE_HASH_KEY` | Pseudonymizes identifiers in Redis history keys | History keys change; every user looks new until history rebuilds |

A secret must be rotatable on a schedule and after a suspected exposure
([INCIDENT_RESPONSE.md](../INCIDENT_RESPONSE.md#suspected-key-compromise)) without an outage,
and before the first production customer, because the first rotation is the worst time to learn
it does not work ([DEPLOYMENT.md](../DEPLOYMENT.md#secrets)).

## Decision

Each secret gets one optional companion, `<NAME>_PREVIOUS`, read at start-up. Signing and
deriving always use the current value. The previous value is used only to verify or to derive
what was issued under it. A rotation is: set `_PREVIOUS` to the old value and the main variable
to the new one, deploy, wait for the retirement condition, then remove `_PREVIOUS`.

1. **API key pepper.** Verification compares against the current pepper and then the previous
   one, in constant time. A match against the previous pepper rehashes the stored secret under
   the current pepper in the same request, so the population migrates by use. Rehash failure
   never fails authentication. A counter, `riskos.auth.previous_pepper`, records matches.
   Retirement: the counter has been zero for a full business cycle (14 days); keys that were
   never used in that time are still on the old pepper and stop working when it is removed,
   which is the intended effect for dormant keys and is announced to tenants beforehand.
2. **Webhook signing key.** Customers hold derived secrets, so the old and new secrets must be
   valid together. Rotation of the master key is a bulk rotation of every endpoint: each
   endpoint's secret version is raised, the previous version is kept valid for the overlap
   period (the mechanism per-endpoint rotation already has), and the previous version is derived
   with `RISKOS_WEBHOOK_SIGNING_KEY_PREVIOUS`. Deliveries carry both signatures during the
   overlap; customers fetch the new secret from the console and switch. After the overlap the
   previous key is removed. The bulk rotation is a staff operation (`riskosctl`), audited per
   endpoint, with an overlap of at least 7 days.
3. **Console secret.** Tokens are verified against the current secret and then the previous
   one, and are always issued under the current one. Retirement: after the longest token
   lifetime (session maximum), when no token issued under the old secret can still be live.
4. **Feature hash key.** There is no way to keep history across a change, because the keys are
   one-way. Rotation is therefore a planned event: history signals warm up again, which the
   engine already reports (warming signals) and decisions are weaker, not wrong
   ([DISASTER_RECOVERY.md](../DISASTER_RECOVERY.md#losing-redis-state)). It is done in a quiet
   period, announced, and never on a schedule shorter than a year unless exposure requires it.
   No `_PREVIOUS` variable is provided for it.
5. **Start-up checks.** A `_PREVIOUS` value must meet the same minimum length and must differ
   from the current value; production start-up refuses otherwise. The process never logs either
   value, and records only that a previous key is configured.
6. **Procedure.** Each rotation is a runbook step list in
   [DEPLOYMENT.md](../DEPLOYMENT.md#secrets), exercised in staging before production use.

## Alternatives considered

- **Key identifiers stored with each hash (`kid`).** The standard approach and the right one
  for many keys, but it needs a schema change on every API key and endpoint row and a backfill.
  With one active and one previous key, trying both is equivalent and needs no migration.
  Revisit if rotations overlap or keys need to be revoked individually.
- **Envelope encryption with a master key in a KMS.** Moves the problem to the platform, which
  is undecided, and does not help API key hashes, which are one-way by design.
- **Rotate by re-issuing every credential.** Correct after a compromise, but it forces every
  customer to act at once; it remains the emergency path, not the routine one.

## Consequences

- Rotation becomes routine for three of the four secrets and a planned event for the fourth.
- Authentication does one extra HMAC for keys still on the previous pepper, only during a
  rotation.
- A previous secret that is left configured keeps accepting what it accepted; the start-up
  check and the retirement counters exist so that is visible, and the runbook makes removal an
  explicit step.
- Implemented: the API key pepper (decision 1): `RISKOS_API_KEY_PEPPER_PREVIOUS`, migration
  `00033_api_key_rehash.sql` (a compare-and-swap `SECURITY DEFINER` function, the second
  pre-authentication exception after `auth_lookup_api_key`), and the
  `riskos.auth.previous_pepper` metric. `TestAPIKeyPepperRotation` walks the procedure.
- Follow-ups: the previous-key paths for the console secret and the webhook signing key, and the
  `riskosctl` bulk webhook rotation (decisions 2 and 3); the start-up warning that a previous
  key is still configured; exercise each rotation in staging.
