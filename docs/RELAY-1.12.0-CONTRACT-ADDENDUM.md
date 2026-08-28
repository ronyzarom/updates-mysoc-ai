# Relay 1.12.0 Contract Addendum — Heartbeat Interval Hint (v1)

Status: **authored by the updates/relay team**, for countersign by the
SWF/updater team before the child-facing side ships. This addendum extends the
[Relay 1.11.0 Contract](RELAY-1.11.0-CONTRACT.md) with one **additive, opt-in**
field introduced by Fleet Scalability 1.12. It requires **zero** SWF changes to
keep working; honoring the hint is an optional client enhancement.

Goal: at 20,000 customers per operator a mysoc relay fronts ~20k customer
relays. Letting the relay *advise* a longer heartbeat cadence flattens the
request rate at the busiest hop without hard-coding an interval into every
leaf. The hint is advice, never a command.

---

## The field (normative)

A 1.12.0+ relay (and the updates server) MAY include one additional field in
any heartbeat response:

```json
{
  "status": "ok",
  "updates": [],
  "relay_token": "rt_...",
  "identity": { "parent_instance_id": "siemcore-testing-01" },
  "ack_cursor": 4821,
  "heartbeat_interval_seconds": 300
}
```

- `heartbeat_interval_seconds` is a positive integer: the interval the parent
  would prefer this child heartbeat at. Absent or `0` means "no preference —
  keep your configured cadence".
- `ack_cursor` is the companion delta-stream acknowledgement (see the delta
  reporting section of the deployment guide); it is unrelated to the interval
  hint and is documented here only because both are new 1.12.0 response fields.

## Relay obligations (normative)

1. The hint is **advisory**. A relay MUST remain correct if a child ignores it
   entirely and keeps heartbeating at its configured interval — the port guard
   and child registry already bound abusive cadences.
2. The relay MUST NOT treat a child that ignores the hint as failing, and MUST
   NOT ban or deprioritize it for cadence alone.
3. The hint is sourced from `relay.child_heartbeat_interval` in the relay
   config. Unset (the default) means the relay sends **no** hint field.

## Client obligations (normative, for a client that opts in)

4. A client MAY honor the hint by adopting it as its heartbeat interval, and
   MAY clamp it to a locally configured `[min, max]` band so a hostile or
   misconfigured relay cannot push it to an unsafe cadence (e.g. never slower
   than 30 min, never faster than 30 s).
5. A client that honors the hint MUST still send an **immediate** heartbeat on
   material local events (enrollment, version change, update attempt,
   decommission) regardless of the advised interval — the interval governs the
   idle keep-alive cadence only, and material changes still flow promptly as
   deltas.
6. A client that does not understand the field MUST ignore it. It is a plain
   additive JSON field; unknown-field tolerance is already required by the
   1.11.0 contract.

## Compatibility

| Client | Relay/Server | Behavior |
|---|---|---|
| new (opts in) | 1.12.0+ with hint configured | client adopts the clamped interval; material changes still immediate |
| new (opts in) | 1.12.0+ no hint, or pre-1.12.0 | no field present; client keeps its configured interval |
| old | 1.12.0+ with hint configured | field ignored; client keeps its configured interval — no regression |

There is no deploy-ordering constraint: the field is ignored by any party that
does not understand it, in either direction.

## Security note

The hint grants the relay no new authority. A relay can already influence a
child's effective cadence indirectly through the port guard's rate limits; this
formalizes a *cooperative* path with an explicit client-side clamp, which is
strictly safer than a leaf guessing a fixed interval or being throttled blindly.

## Changelog

- **v1** — introduces `heartbeat_interval_seconds` as an additive, advisory,
  opt-in heartbeat response field (Fleet Scalability 1.12). No required SWF
  changes; documents the companion `ack_cursor` field for completeness.
