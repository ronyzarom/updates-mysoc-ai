# Relay 1.15.0 Contract Addendum — Product Delivery Telemetry (v1)

Status: **countersigned** — the updates/relay team and the SWF/updater team
aligned on the field set and constraints on **2026-09-05**. This is the
carrier/renderer side of the SWF-side *"Updater contract addendum — SWF
delivery telemetry on the heartbeat"*; the two documents describe the same wire
object from the producer and carrier ends. This addendum extends the
[Relay 1.11.0 Contract](RELAY-1.11.0-CONTRACT.md) with one **additive** object
carried inside an existing heartbeat field. First consumer: the Secure SWF
Updater (2.2.0.15), which reads SWF's `status.ini` (`format_version = 2`) and
attaches delivery counters to its own product entry.

Goal: make "**is this SWF actually sending logs?**" answerable from the node's
fleet-dashboard entry, without a command channel and without host metrics. The
SWF updater already heartbeats every 60 s and SWF already writes the counters;
this only defines the wire shape and the omission rules so the counters survive
the cascade.

---

## The field (normative)

A product entry in `heartbeat.products[]` MAY carry one additional object,
`telemetry`, describing that product's delivery health:

```json
{
  "instance_id": "swf-acme-WS0042",
  "product_tier": "swf",
  "products": [
    {
      "name": "swf",
      "version": "2.2.0",
      "channel": "stable",
      "status": "running",
      "telemetry": {
        "ready": true,
        "connection": "connected",
        "sent": 14820,
        "seen": 14821,
        "admitted": 14820,
        "delivery_eps_milli": 2500,
        "last_write_utc": "2026-09-05T06:40:00Z",
        "spool_events": 0,
        "spool_bytes": 0,
        "status_utc": "2026-09-05T06:40:01Z",
        "last_error": "syslog tls: handshake failure"
      }
    }
  ]
}
```

Fields (all optional; numeric-first, no log content). The **status.ini v2 key**
column is the producer-side source of each value, per the SWF-side addendum:

| Field | Type | status.ini v2 key | Meaning |
|---|---|---|---|
| `ready` | bool | `ready` | SWF reports itself ready to deliver. |
| `connection` | string | `connection_state` | `connected` or `disconnected` to the downstream collector; omitted when empty. |
| `sent` | integer (uint64) | `sent` | Cumulative socket-write count to the collector. **A socket write is not an ingestion acknowledgement** — the name says exactly what it measures. |
| `seen` | integer (uint64) | `seen` | Events observed at intake. |
| `admitted` | integer (uint64) | `admitted` | Events admitted past rate limits / filtering. |
| `delivery_eps_milli` | integer (uint64) | `delivery_rate_milli_eps` | Delivery rate in events/sec × 1000 (integer transport of a fractional EPS; `2500` = 2.5 eps). |
| `last_write_utc` | RFC 3339 UTC | `last_socket_write` | Last time SWF wrote an event downstream; omitted when empty. |
| `spool_events` | integer (uint64) | `queue_count` | Events currently spooled (backpressure/outage). |
| `spool_bytes` | integer (uint64) | `spool_physical_bytes` | Spool physical size in bytes. |
| `status_utc` | RFC 3339 UTC | `status_timestamp` | SWF's own snapshot time for these counters (see freshness); omitted when empty. |
| `last_error` | string (≤ 512 bytes) | `last_error` | Last delivery error, already credential-redacted and length-bounded (≤ 512 bytes) by the agent; omitted when empty. |

The whole `telemetry` object is bounded (**< 1 KiB** worst case) and rides
inside the existing heartbeat payload bound.

The carrier transports the wire integers faithfully; this repo types the
counters as Go `int64` (matching every other counter in `pkg/types`), which is
wire-compatible with the contract's `uint64` for all realistic event counts.

Counters are monotonic since SWF start unless the addendum on the agent side
documents a windowed reset; the dashboard treats them as levels, not rates,
except `delivery_eps_milli`.

## Freshness (`status_utc`)

`status_utc` is the timestamp SWF stamped on the counters, distinct from the
cascade `last_heartbeat`. A consumer MUST use `status_utc` to judge whether the
telemetry is live: a `connection: "connected"` with a stale `status_utc`
(older than a small multiple of the heartbeat interval) reads as **silent**,
not delivering. This prevents a frozen SWF process from looking healthy just
because the updater is still heartbeating.

## Omission semantics (normative)

The `telemetry` object is **omitted entirely** — the key is absent, never an
empty object and never empty strings — when any of these hold:

- SWF is not installed on the host;
- `status.ini` is missing, unreadable, or unparsable;
- the status file is not `format_version = 2`.

This mirrors the 1.11.0 identity-omission convention. Individual **empty-string
fields are omitted one by one** (partial data is allowed — send the numeric
fields you have), but a wholly unavailable source omits the whole key. A
consumer MUST treat a missing key as **"not reported"**, never as "not
delivering". Telemetry collection is **fail-open**: it MUST never fail, delay,
or block a heartbeat. A node that sends no `telemetry` is indistinguishable on
the wire from a pre-2.2.0.15 agent.

## Relay obligations (normative)

1. A relay MUST preserve a child's `products[].telemetry` in its upward rollup.
   The relay copies the child's `products` slice into its `children[]` report
   unchanged; `telemetry` rides along automatically once the relay binary
   understands the field.
2. A relay MUST NOT gate, rate-limit, or reject a heartbeat on the presence,
   absence, or content of `telemetry`. It is pure visibility.
3. `telemetry` participates in change detection only weakly (see Client /
   delta note): a relay in delta-reporting mode MAY treat a change in
   `connection` or `last_error` as material, but MUST NOT emit a delta on every
   counter tick, or steady delivery would defeat change-only reporting.

## Server obligations (normative)

4. The server stores the telemetry as part of the instance's
   `last_heartbeat_data`. No new column or migration is required. **The pipeline
   is not pass-through**: each hop unmarshals into the Go `Heartbeat` structs and
   re-marshals, so `last_heartbeat_data` holds the **re-encoded typed struct,
   not the producer's original bytes** — which is exactly why `ProductStatus`
   must carry a typed `Telemetry` field (see Compatibility).
5. Rendering is additive: a server/dashboard that does not understand
   `telemetry` ignores it. **First target — the instance detail page** (the
   fleet list/tree queries deliberately omit `last_heartbeat_data`): it derives
   a **delivering / silent / stopped** signal from the product `status`,
   `telemetry.connection`, `telemetry.sent`, and `telemetry.status_utc`
   freshness. A fleet-list badge is explicitly **later, separate work**.

## Client obligations (normative)

6. The agent attaches `telemetry` to the product entry it owns, matched by
   `name` (`"swf"`), not by array position.
7. The agent redacts and length-bounds `last_error` before sending. No log
   lines, paths, or payloads.
8. The agent honors the omission semantics above; partial data is allowed
   (send the numeric fields you have), but a wholly unavailable source omits
   the key.

## Compatibility and deploy ordering

Unlike the 1.12.0 heartbeat-interval hint (a downward, ignore-if-unknown
field), `telemetry` travels **upward through relays that decode and re-encode
each heartbeat into typed structures**. A relay or server binary that predates
the `telemetry` type **silently drops the object at decode** — it is not an
error, but the data does not appear.

Therefore there **is** a soft deploy-ordering constraint:

| SWF agent | siemcore relay / mysoc relay / server | Result |
|---|---|---|
| pre-2.2.0.15 | any | no `telemetry` sent; unchanged behavior |
| 2.2.0.15 | pre-1.15.0 on any hop | `telemetry` dropped at the first old hop; node still healthy, just no delivery panel |
| 2.2.0.15 | 1.15.0+ on every hop to the server | `telemetry` reaches `last_heartbeat_data`; dashboard can render it |

No party fails or regresses in any combination; the only effect of an old hop
is that the panel stays empty. Roll the cascade updaters (mysoc + siemcore) and
the server to 1.15.0+ before, or together with, the SWF 2.2.0.15 rollout so the
data is visible the moment SWF starts sending it.

## Security note

`telemetry` grants no new authority in either direction. It is agent-authored,
read-only visibility carried on an already-authenticated heartbeat; a relay
never acts on it beyond forwarding, and the server never acts on it beyond
storage and display.

## Changelog

- **v1.1 (2026-09-05)** — countersigned with the SWF-side addendum: added the
  `status.ini v2` source-key column, the `last_error` ≤ 512-byte bound and
  `< 1 KiB` object bound, explicit per-field empty-string omission and
  "missing = not reported" rule, the `last_heartbeat_data` re-encode note, and
  the instance-detail-first rendering scope (fleet badge deferred). No wire or
  code change from v1 — the shipped 1.15.0.1 carriers already conform.
- **v1** — introduces the additive `products[].telemetry` object (SWF delivery
  counters), its omission semantics, `status_utc` freshness rule, and the soft
  deploy-ordering note for the decode/re-encode cascade path.
