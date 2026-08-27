# Relay 1.11.0 Contract — Identity Adoption and Decommission (v1)

Status: **authored by the updates/relay team**, for countersign by the
SWF/updater team before either side ships. This is the final contract text
for the two items proposed in `RELAY-1.11.0-CONTRACT-PROPOSAL.md` (SWF team,
2026-08-27); the proposal's mechanism is adopted with three amendments,
marked **[A1] [A2] [A3]** below. First consumers: the Secure SWF Updater
(next 2.2.0.x build after countersign) and the `siemcore-swf-setup`
bootstrap. The relay and server implementation ships in platform 1.11.0.

Goal, restated from the proposal: **a customer needs to know exactly one
value — which SiemCore host to use.**

---

## Item A — Enrollment-time identity adoption

### Relay obligations (normative)

1. Every heartbeat response from a 1.11.0+ relay carries an `identity`
   object alongside the existing fields:

   ```json
   {
     "status": "ok",
     "updates": [],
     "relay_token": "rt_...",
     "identity": {
       "parent_instance_id": "siemcore-testing-01",
       "customer_id": "siemcore-internal",
       "customer_name": "SiemCore Internal (Testing)"
     }
   }
   ```

   `parent_instance_id` is always present and is the relay's own instance
   id. `customer_id` and `customer_name` are present only when configured
   on the relay.

2. **[A1] Partial identity.** The relay returns what it knows and omits
   what it does not. Fields are never sent as empty strings; a relay with
   no configured customer identity sends an `identity` containing only
   `parent_instance_id`. The client adopts the fields that are present.

3. The relay MAY reject enrollment for nodes it does not recognize by
   policy (capacity, guard bans); this surfaces to the client identically
   to today's behavior. Identity is attestation, not authorization: the
   server-side authorization gates (auto-update flag, update group) are
   unchanged by this contract.

### Client obligations (normative)

4. **First contact without identity.** A node with no adopted or
   explicitly configured identity sends its first heartbeat with the
   identity fields **absent** (not empty strings). All other fields are
   unchanged: `instance_id`, `instance_type`, `product_tier`, `hostname`,
   `updater_version`, `products`, device-secret header, absent
   `X-Relay-Token`.

5. **Adopt once, echo forever.** The client persists adopted identity in
   protected machine state alongside the device secret and relay token,
   and includes it in every subsequent heartbeat, check, and report
   payload exactly as an explicitly configured client does today. The
   server-side requirement that check payloads carry `product_tier` and
   `parent_instance_id` is unchanged; only the source of the values moves.

6. **Adoption is one-shot and static.** A later response carrying a
   *different* `identity` never mutates persisted state; the client keeps
   its values and surfaces a local warning (the 401 doctrine applied to
   identity).

7. **Explicit values win.** Identity flags remain supported. Precedence:
   explicit flags → previously adopted/persisted identity → adoption.

8. **Pre-1.11.0 relay** (response carries no `identity`): the client
   fails early and clearly at preflight — before any download — naming
   the three explicit flags. Nothing is half-installed.

### Security note (agreed)

Relay-attested identity formalizes the trust model the cascade already
runs on: the server already derives every rollup row's
`parent_instance_id` from the relay's reported tree position, ignoring
the child's declared value. This contract extends the same attestation to
the child's own echoed payloads and to customer identity, and grants the
relay no authority it does not already have.

---

## Item B — Decommission

### Endpoint (normative)

```
POST /api/v1/decommission
Headers: X-License-Key: <device secret>   (required)
         X-Relay-Token: <relay token>     (required for known children)
Body:    {"instance_id": "<id>"}
```

Responses:

- `200 {"status":"decommissioned"}` — marked, **idempotent**: repeat calls
  and calls for already-decommissioned or unknown instance ids all return
  200. A goodbye is never an error worth retrying into a ban.
- `401` — missing credential or relay-token mismatch (counts toward the
  port guard's auth-failure ladder like any other route).
- `404` — pre-1.11.0 relay (no such route). The client treats this as
  unreachable and fails open.

**[A3] Guard applicability.** The endpoint sits behind the relay's port
guard like every child route: per-IP rate limits and temp-bans apply. The
client's single best-effort call with a short bounded timeout (no retry
loop inside uninstall) is compatible with this by construction.

### Semantics (normative)

- **State, not deletion.** Decommission marks the instance; it never
  deletes. The audit trail (row, history, last heartbeat data) is
  preserved. Hard deletion remains an operator-only, admin-credentialed
  dashboard action.
- **Propagation rides the rollup.** The relay marks the child locally and
  acks immediately; the mark reaches the updates server as a
  `decommissioned` child status in the relay's next upward heartbeat —
  the same path that carries online/offline today. It works at any
  cascade depth and requires no new upstream call.
- **Unknown children are still marked.** If the relay has restarted and
  forgotten the child, the decommission call creates a tombstone entry so
  the status still rolls up; the server merges it into the existing row.
- **Revival is visible and honest.** A subsequent genuine heartbeat from
  the instance (e.g. reinstall without `--purge`, or a stolen-credential
  false decommission) flips the row back to online through the normal
  contact path. Decommissioned is only terminal when the node is
  genuinely gone.
- **Relay retention.** Decommissioned entries are pruned from relay state
  after 7 days (the server row is the durable record); relay restarts do
  not lose un-delivered marks (tombstones persist in the relay cache
  directory).
- **Direct nodes.** Tier-1 nodes that heartbeat straight to the updates
  server use the same endpoint on the server, authenticated like their
  heartbeat (license key required).

### Consumer expectations (normative, from the SWF proposal)

1. **Call placement and credential lifetime.** The client calls the
   endpoint after managed product removal succeeds and **before** the
   updater removes its own service and state — the last moment it still
   holds credentials. One best-effort call, short bounded timeout, no
   retry loop; fail-open: local uninstall always succeeds regardless of
   network state.
2. **Retention interplay.** Without `--purge`, retained state keeps
   identity and credentials: a missed decommission is retriable later,
   and a subsequent reinstall legitimately revives the same row. With
   `--purge`, credentials are destroyed with the state: if the call
   failed, operator delete is the only remaining path.
3. **Idempotency and mode independence.** Decommissioning an
   already-decommissioned instance acks. The endpoint is callable
   regardless of update mode — none of the fail-closed update gates apply
   to a goodbye message. (All child-facing relay traffic is TLS-only
   since 1.9.0.)
4. **First consumer wiring.** One best-effort call in the
   `uninstall`/`uninstall-swf` path, no flag, no config; 404 on an older
   relay looks like unreachable and fails open.

### Dashboard (informative)

Decommissioned instances render distinctly (neutral styling, explicit
label), are excluded from offline/unhealthy alarms, and can be deleted by
an admin as today. Liveness derivation never overrides a stored
`decommissioned` status; only a new heartbeat does.

---

## [A2] Sequencing (hard requirement)

The pre-1.11.0 server coerces unknown rollup child statuses to `online`.
A relay that emits `decommissioned` toward an older server would therefore
make cleanly removed nodes appear online. Deploy order is mandatory and is
the updates team's to guarantee:

1. Updates server 1.11.0 (accepts and persists `decommissioned`,
   direct-node endpoint, dashboard rendering).
2. Relay kits 1.11.0 (identity object, decommission endpoint, rollup
   status).
3. Updater-side consumers (SWF 2.2.0.x bootstrap and uninstall wiring)
   after countersign, in any order relative to each other.

Item A has no ordering constraint: an `identity` object from a new relay
is ignored by old clients, and its absence on an old relay is the
documented fail-early path for new clients.

## Compatibility matrix (carried from the proposal)

| Client | Relay | Install | Uninstall |
|---|---|---|---|
| new | 1.11.0+ | `--host` only (identity adopted at preflight) | decommission marked on dashboard |
| new | pre-1.11.0 | identity flags required; fail-early with clear message | row goes silent as today (fail-open 404) |
| old | 1.11.0+ | unchanged (explicit identity) | row goes silent as today |

## Changelog

- **v1** — adopted from the SWF team's proposal with three amendments:
  [A1] partial identity semantics; [A2] server-before-relay deploy order
  (status coercion on pre-1.11.0 servers); [A3] port-guard applicability
  and removal of the "plaintext-relay" wording (child listener is
  TLS-only since 1.9.0).
