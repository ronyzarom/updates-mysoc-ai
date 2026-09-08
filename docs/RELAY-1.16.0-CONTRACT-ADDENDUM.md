# Relay 1.16.0 Contract Addendum — Product Delivery Destination (v1)

Status: **proposed for countersign** by the SWF/updater team (producer). The
updates/relay team (carrier/renderer) side is implemented in 1.16.0. This
addendum extends the
[Relay 1.15.0 Contract Addendum](RELAY-1.15.0-CONTRACT-ADDENDUM.md) with five
**additive, optional** fields inside the existing `products[].telemetry`
object. No new object, no new endpoint, no migration.

Goal: when a node reads **silent** or **stopped**, answer the next question
from the same detail page — **where is this SWF trying to deliver?** Observed
live (DC01/DC02/FS02, 2026-09-05): `last_error` said `connect ... 10060` and
`10053`, but nothing on the DevOps channel said *to which host:port*, so
"customer firewall drops outbound 6514 to the right collector" and "agent
points at the wrong collector" were indistinguishable without the customer.

---

## Why on the DevOps channel (reconciliation with the SOC contract)

The SWF ↔ SOC control contract (`SWF-SOC-CONTROL-CONTRACT.md`, owned by the
siemcore team; not in this repo) separates two channels: **DevOps** (updater lifecycle: heartbeat, versions, self-update) and
**SOC** (log delivery, inventory, control). Delivery configuration is a SOC
concern, and this addendum does **not** move it: the DevOps channel never sets,
changes, or validates the destination. It only **reports** what the agent
already knows about itself.

The justification is the failure mode this addendum exists for: when SWF
cannot reach its collector, the **SOC channel is dark by definition** — the SOC
cannot read a configuration from an agent that is not reaching it. The DevOps
channel is the independent path that still works in that state, so it is the
only place an operator can see *why* delivery fails. That makes these fields
**out-of-band diagnostics**, read-only and display-only, not a second control
plane. Nothing in the cascade acts on them beyond forwarding, storage, and
display.

## The fields (normative)

All five live inside `products[].telemetry` on the entry whose `name` is
`"swf"`, beside the 1.15.0 counters:

```json
{
  "name": "swf",
  "version": "2.2.0",
  "channel": "stable",
  "status": "running",
  "telemetry": {
    "connection": "disconnected",
    "sent": 14820,
    "spool_events": 3120,
    "status_utc": "2026-09-05T06:40:01Z",
    "last_error": "connect cyfox-il.siemcore.ai:6514: 10060",
    "target_endpoint": "cyfox-il.siemcore.ai:6514",
    "target_resolved_ip": "34.165.118.36",
    "target_tls": true,
    "target_sni": "cyfox-il.siemcore.ai",
    "last_connect_ok_utc": "2026-09-04T22:11:07Z"
  }
}
```

| Field | Type | status.ini v2 key | Meaning |
|---|---|---|---|
| `target_endpoint` | string | `target_endpoint` | The configured delivery destination as `host:port` exactly as SWF will dial it (hostname or literal IP). Omitted when unset. |
| `target_resolved_ip` | string | `target_resolved_ip` | The address `host` last resolved to on the agent (empty / omitted when resolution failed or the host is a literal IP). Lets the operator see a DNS problem or a split-horizon answer. |
| `target_tls` | bool | `target_tls` | `true` when SWF delivers over TLS. **Omitted when false** (see caveat). |
| `target_sni` | string | `target_sni` | TLS server name presented in the handshake, when it differs from or supplements `host`. Omitted when empty or when TLS is off. |
| `last_connect_ok_utc` | RFC 3339 UTC | `last_connect_ok` | Last time a TCP/TLS connection to `target_endpoint` completed successfully (not a write, not an ack). Omitted when SWF has never connected since install. |

The five fields add well under 256 bytes; the whole `telemetry` object stays
inside the 1.15.0 **< 1 KiB** bound.

### `target_tls` caveat (normative)

`target_tls` is a boolean serialized with `omitempty` (the same treatment as
`ready`), so **`false` does not appear on the wire**. A consumer MUST NOT infer
"TLS off" from a missing `target_tls` alone; it reads "TLS off" **only when
`target_endpoint` is present and `target_tls` is absent** — i.e. the agent is
reporting its destination and did not claim TLS. When `target_endpoint` is also
absent, the whole destination block is "not reported" (pre-1.16.0 agent).

### Timestamps are omitted, not zeroed (carrier fix)

1.15.0 carriers typed `last_write_utc` / `status_utc` as Go `time.Time`, whose
zero value is **not** omitted by `omitempty`; an agent that left them out could
therefore reappear downstream as `"0001-01-01T00:00:00Z"` after one re-encode.
1.16.0 carriers type all three telemetry timestamps (`last_write_utc`,
`status_utc`, `last_connect_ok_utc`) as nullable so an omitted timestamp
**stays omitted through every hop**. The dashboard additionally treats a
`0001-01-01…` string as absent so a 1.15.0 relay left mid-cascade cannot
render "about 2000 years ago". Wire format when present is unchanged.

## Omission semantics (normative)

Same rules as 1.15.0, applied per field:

- Each string / timestamp field is omitted individually when empty.
- `target_tls` is omitted when false.
- If the agent cannot read a destination at all (SWF absent, config
  unreadable, status file pre-dates these keys), it omits **all five**; the
  1.15.0 counters are unaffected.
- A consumer MUST treat a missing `target_endpoint` as **"destination not
  reported"**, never as "no destination configured".
- Fail-open: collecting these fields MUST never fail, delay, or gate a
  heartbeat.

## Relay obligations (normative)

1. A relay MUST preserve the five fields in its upward rollup. They ride
   inside the same typed `ProductTelemetry` the relay already copies; a relay
   compiled without them strips them at decode exactly as pre-1.15.0 relays
   stripped the whole `telemetry` object.
2. A relay MUST NOT act on the destination: no connectivity probe, no
   validation, no rewrite, no gating. It is pure visibility.
3. In delta-reporting mode a relay MAY treat a change of `target_endpoint` or
   `target_resolved_ip` as material (a destination change is rare and
   diagnostic), but MUST NOT emit a delta on `last_connect_ok_utc` alone.

## Server obligations (normative)

4. Stored inside `last_heartbeat_data` with the rest of `telemetry`. **No new
   column, no migration.**
5. Rendering: the instance detail page Products→swf telemetry block shows a
   **Destination** row (`target_endpoint`, with `target_resolved_ip` in
   parentheses when present), a **TLS** row (`on · <sni>` / `off`, only when
   `target_endpoint` is present), and a **Last connect OK** row (relative time,
   only when present). The delivering / silent / stopped signal from 1.15.0 is
   unchanged; these rows explain it, they do not feed it.
6. The server MUST NOT expose a way to change the destination through the
   DevOps API. The SOC channel owns configuration.

## Client obligations (normative)

7. `target_endpoint` is the value SWF actually dials — after any config
   normalisation, before any DNS resolution.
8. `target_resolved_ip` is what the agent's resolver returned; the agent does
   not resolve separately for telemetry (no extra DNS traffic per heartbeat).
   It is empty when the last resolution failed.
9. `last_connect_ok_utc` is stamped on successful connect (and TLS handshake
   when `target_tls`), never on a queued write.
10. No credentials, no certificates, no log content. `target_sni` is a
    hostname only.

## Compatibility and deploy ordering

Same soft constraint as 1.15.0 — the fields travel upward through hops that
decode and re-encode into typed structs:

| SWF agent | siemcore relay / mysoc relay / server | Result |
|---|---|---|
| pre-destination (≤ 2.2.0.15) | any | five fields absent; 1.15.0 behavior |
| destination-capable | pre-1.16.0 on any hop | five fields dropped at the first old hop; 1.15.0 counters still arrive; no Destination rows |
| destination-capable | 1.16.0+ on every hop to the server | Destination / TLS / Last connect OK render |
| any | 1.16.0+ carriers, 1.15.0 relay mid-cascade | five fields dropped there; zero-time guard on the dashboard still applies |

No combination fails or regresses. Roll the **updates server**, the
**mysoc-updater**, and the **siemcore-cascade-updater** to 1.16.0+ before or
together with the destination-capable SWF updater so the rows appear the moment
the agent starts sending them.

## Security note

The five fields grant no new authority in either direction. They expose the
agent's own delivery target (hostname, port, resolved IP, SNI) to the operator
who already owns the collector the agent points at; no credentials or
certificates travel. The updates server cannot change the destination and has
no endpoint to do so.

## Changelog

- **v1 (2026-09-08)** — introduces `target_endpoint`, `target_resolved_ip`,
  `target_tls`, `target_sni`, `last_connect_ok_utc` inside
  `products[].telemetry`; `target_tls` omitempty caveat; nullable telemetry
  timestamps on carriers; DevOps-vs-SOC reconciliation note; deploy-ordering
  table. Proposed for countersign by the SWF/updater team.
