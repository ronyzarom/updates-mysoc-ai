# Relay 1.14.1 Contract Addendum — Reject Reason Code on 401 (v2)

Status: **authored by the updates/relay team**, for countersign by the
SWF/updater team. This addendum extends the
[Relay 1.11.0 Contract](RELAY-1.11.0-CONTRACT.md) with one **additive** field on
existing `401 Unauthorized` responses from the relay's child-facing endpoints.
It requires **zero** SWF changes to keep working; the field is an **operator
diagnostic**, not a client feature.

Goal: make the relay's token reject self-explanatory. A `401` used to be an
opaque `relay token mismatch` for two very different situations — an empty token
against a live binding, versus a wrong token. The new `code` field and a
structured relay-side log tell them apart, so an operator can see at a glance
whether a reject is a benign process-ordering artifact on the host or a genuine
identity conflict worth investigating.

The empty-token case has a specific meaning that is central to this addendum:
it can only occur when a control-plane call is made **without** the updater
service's persisted token. The service instance always presents that token, so
an empty token against a live binding means an **auxiliary invocation of the
updater binary** (e.g. a `preflight` verb — not the service instance or a
deliberate operator verb) reached the control plane before enrollment persisted.
The durable fix is to keep such verbs local-only (see Trust boundary below), not
to retry. The leaf application is never involved — it holds no credentials.

---

## The field (normative)

A 1.14.1+ relay MAY include one additional field in the JSON body of a `401`
from any child-facing endpoint (`/api/v1/heartbeat`, `/api/v1/decommission`,
`/api/v1/updates/{product}/check`, `/api/v1/updates/{product}/report`, and the
release metadata/download routes):

```json
{
  "error": "relay token mismatch for this instance_id",
  "code": "relay_token_absent"
}
```

`code` is one of:

- `relay_token_absent` — the relay holds a **live token binding** for this
  `instance_id`, but the request presented **no** `X-Relay-Token`. Since the
  updater service instance always presents its persisted token, an empty token
  against a live binding means an **auxiliary updater invocation** (e.g. a
  `preflight` verb — not the service instance or a deliberate operator verb)
  reached the control plane before enrollment persisted — a first-contact race.
  **Signals a host-side boundary violation to fix at the source, not a flow to
  retry.**
- `relay_token_mismatch` — a **non-empty** `X-Relay-Token` was presented that
  does **not** match the binding. Typical cause is a stale/incorrect token or a
  different node reusing the `instance_id`. **Investigate; never auto-retry.**

The `error` message is unchanged and remains the human-readable string. Absence
of `code` (any pre-1.14.1 relay) means "reason unspecified" — treat as
non-retryable.

## Trust boundary (normative)

The relay's child principal is the **updater** (`mysoc-updater` /
`siemcore-cascade-updater`). The leaf application (SWF) it delivers is a payload
that has never held or presented credentials and is out of scope here. The
boundary that matters is therefore *within* the updater: credential presentation
is scoped to specific actors.

1. The **updater service instance is the sole routine credential presenter**: it
   reads, persists, and presents the relay token and license key on its
   heartbeat cadence, and it is the party the relay's live binding tracks.
2. **Deliberate operator verbs** (e.g. `provision --yes`, `heartbeat`, `apply`)
   are also legitimate control-plane presenters — operator-initiated and
   retry-protected. They are expected to carry credentials.
3. **Auxiliary invocations stay off the control plane.** A non-operator verb of
   the updater binary (e.g. `preflight`) MUST confirm state **locally only** —
   token persisted, identity attributed, last service status surfaced on
   failure, with a bounded wait for first enrollment — and MUST NOT present a
   token to a relay.
4. The **leaf application** neither holds nor presents credentials and needs no
   awareness that either exists; if it needs enrollment status it obtains that
   from the local updater, never from a relay.
5. Consequently, once auxiliary invocations are control-plane-free, a
   `relay_token_absent` from any install path is **structurally impossible**.
   Its appearance in a relay journal is precisely the signal that an auxiliary
   (non-service, non-operator) invocation reached the control plane — a
   host-side boundary violation to fix at the source, not a flow to retry.

## Relay obligations (normative)

1. The relay MUST NOT change the status code: both reasons remain `401`.
2. The relay MUST NOT weaken first-contact anti-hijack to satisfy a retry: an
   empty token against a live binding is still rejected. The code is a
   diagnostic and retry hint only, never an authorization bypass.
3. The relay's per-IP guard (ban ladder) continues to apply to repeated
   failures, so a retry loop that never re-reads a valid token is still bounded.
4. These codes describe **live** bindings only. A decommissioned binding
   re-binds on the next honest heartbeat (Relay 1.14.0.1), so a purged reinstall
   does not reach this path.

## Updater obligations (normative)

These apply to the updater's **control-plane actors** — the service instance and
deliberate operator verbs. Auxiliary invocations do not present tokens at all
(Trust boundary §3), so these do not apply to them.

5. The updater service SHOULD serialize its own first contact — persist the
   token before any co-running updater process can read the credential store —
   so it never presents an empty token against its own live binding. If a
   control-plane actor nonetheless sees `code == "relay_token_absent"`, it MAY
   re-read the persisted credential and retry **once**, then back off rather than
   tight-loop (the per-IP guard otherwise bans the source).
6. On `code == "relay_token_mismatch"`, a control-plane actor MUST NOT silently
   retry with the same or an empty token; this indicates a stale token or an
   `instance_id` collision and needs operator attention.
7. A party that does not understand `code` MUST ignore it and treat the `401` as
   before. It is a plain additive JSON field; unknown-field tolerance is already
   required by the 1.11.0 contract.

## Operator diagnostic (informative)

`relay_token_absent` is primarily a **forensic signal**, not a retry trigger.
Read against the relay's `child enrolled at relay` log line, a
`… reason=relay_token_absent` for the same `instance_id` seconds later is the
fingerprint of an **auxiliary updater invocation** reaching the control plane
before enrollment persisted. The durable fix is to keep such invocations
local-only (per the Trust boundary): confirm enrollment from the updater's own
persisted state, not by calling a relay. The service instance's one-shot retry
(obligation 5) only masks a race *inside* the updater's control-plane actors; it
is not a substitute for keeping auxiliary verbs off the control plane.

## Compatibility

| Party | Relay | Behavior |
|---|---|---|
| service instance / operator verb (understands `code`) | 1.14.1+ | presents persisted token; reads `code` for diagnostics; own first-contact self-race self-heals via one re-read+retry |
| control-plane actor | pre-1.14.1 | no `code` field; 401 treated as non-retryable — no regression |
| any older party | 1.14.1+ | `code` ignored; identical 401 behavior as before |
| auxiliary invocation (local-only) | any | never contacts the control plane, so never elicits `relay_token_absent` — the target state |

There is no deploy-ordering constraint: the field is ignored by any party that
does not understand it, in either direction.

## Changelog

- **v2** — reframes the field as an **operator diagnostic** and adds the **Trust
  boundary** section. Scopes credential presentation *within the updater*: the
  service instance is the sole routine presenter, deliberate operator verbs are
  legitimate retry-protected presenters, and auxiliary invocations (e.g. a
  `preflight` verb) stay off the control plane (local confirmation only). The
  leaf application holds no credentials and is out of scope — it was never the
  racing party. `relay_token_absent` is therefore the signature of an auxiliary
  invocation crossing the boundary, structurally impossible once such verbs are
  control-plane-free, not a client retry flow.
- **v1** — introduces the additive `code` field on relay child-endpoint `401`
  responses (`relay_token_absent` / `relay_token_mismatch`) plus a structured
  relay-side log on the reject path, so the token reject is distinguishable.
