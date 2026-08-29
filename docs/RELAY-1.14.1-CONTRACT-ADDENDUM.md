# Relay 1.14.1 Contract Addendum — Reject Reason Code on 401 (v1)

Status: **authored by the updates/relay team**, for countersign by the
SWF/updater team. This addendum extends the
[Relay 1.11.0 Contract](RELAY-1.11.0-CONTRACT.md) with one **additive** field on
existing `401 Unauthorized` responses from the relay's child-facing endpoints.
It requires **zero** SWF changes to keep working; branching on the code is an
optional client enhancement that makes the first-contact race self-healing.

Goal: a brand-new leaf can hit a `relay token mismatch` on a clean first install
if two client processes race first contact — the enrolling process binds a token
`T`, and a second process (e.g. a setup preflight) presents an **empty** token
before `T` is persisted. That reject is retryable; a genuine wrong-token
mismatch is not. This addendum lets the client tell the two apart.

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
  `instance_id`, but the request presented **no** `X-Relay-Token`. Typical cause
  is a first-contact race: a second process read the credential file before the
  enrolling process persisted the token. **Retryable.**
- `relay_token_mismatch` — a **non-empty** `X-Relay-Token` was presented that
  does **not** match the binding. Typical cause is a stale/incorrect token or a
  different node reusing the `instance_id`. **Not auto-retryable; investigate.**

The `error` message is unchanged and remains the human-readable string. Absence
of `code` (any pre-1.14.1 relay) means "reason unspecified" — treat as
non-retryable.

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

## Client obligations (normative, for a client that opts in)

5. On `401` with `code == "relay_token_absent"`, a client SHOULD **re-read the
   persisted credential** and retry **once**. If the token is now present, the
   retry succeeds; if it is still absent, the enrolling process has not finished
   — back off and retry later rather than tight-looping (the guard will
   otherwise ban the source IP).
6. On `401` with `code == "relay_token_mismatch"`, a client MUST NOT silently
   retry with the same or an empty token; this indicates a stale token or an
   `instance_id` collision and needs operator attention.
7. A client that does not understand `code` MUST ignore it and treat the `401`
   as before. It is a plain additive JSON field; unknown-field tolerance is
   already required by the 1.11.0 contract.

## Recommended client mitigation (informative)

The durable fix for the race lives in the client: order the preflight **after**
confirmed enrollment persistence, or apply obligation (5) — one retry after a
credential re-read. Either removes the spurious `relay_token_absent` on slow
links, which is where the race is observable (fast round-trips hide it).

## Compatibility

| Client | Relay | Behavior |
|---|---|---|
| new (opts in) | 1.14.1+ | branches on `code`; `relay_token_absent` self-heals after one re-read+retry |
| new (opts in) | pre-1.14.1 | no `code` field; client treats 401 as non-retryable — no regression |
| old | 1.14.1+ | `code` ignored; identical 401 behavior as before |

There is no deploy-ordering constraint: the field is ignored by any party that
does not understand it, in either direction.

## Changelog

- **v1** — introduces the additive `code` field on relay child-endpoint `401`
  responses (`relay_token_absent` / `relay_token_mismatch`) plus a structured
  relay-side log on the reject path, so the first-contact race is
  distinguishable and self-healing. No required SWF changes.
