# Exact .33 compatibility refusal review

Reviewed source: SiemCore commit 6852e107b9e9a4ffec3aa51c6b8ae36d40ac88dd.
Candidate archive d81a9cf86ebd8a1b81b1c9e962eb5f76e931ddd66faf3c15a76628df42a4e6df.
This is source-path analysis, not a general proof that the host was unchanged.

The historical B traceback identifies pod_update.py apply line 369, the missing
active-controller maintenance-resume capability check. For the completed
bootstrap path:

- updater/apply sources roles/update-helpers.sh, which defines shell functions;
  none of its mutation functions is invoked before this refusal. Policy reads
  select pod B, greenfield.py --requested returns no from the protected complete
  bootstrap journal; its imports define functions/classes without provisioning.
- Coordinator initialization reads manifest, protected policy/config and machine
  identity. validate performs input validation, not resource provisioning.
- With no product transaction, apply obtains status, public health and peer
  status before rejecting controller_version. Local installed-version lookup,
  dependency checks, role-A quorum-auth configuration, prepared journal, begin,
  stage and runtime installation follow this check and are not reached.
- pod-maintenance dispatch occurs before normal application startup in Go main.
  The status branch constructs clients/readers; NewEtcdAuthority allocates an
  in-memory object and does not grant leases or write keys. QuorumReader.Read
  performs one etcd transaction consisting only of three OpGet operations.
- Peer verification uses existing TLS material and pairing state. PairingStore
  Status uses withState(true): O_RDONLY on existing pairing.lock, shared flock,
  reads pairing.json, and explicitly refuses any non-nil write result. It does
  not create a lock file. Peer HTTP requests are GET /v1/pairing/status and GET
  /v1/runtime/status. Agent handlers read state/runtime files and encode results.
- Public /health calls readinessCheck: database Health is connection Ping;
  Redis health reads cached atomics. It performs no application DDL/data writes.
- The exact compensation refusal occurs before loading/saving a candidate state
  or pause_failure; therefore it cannot justify a paused receipt or prior rollback.

Conclusion: the reviewed exact path does not execute application installation,
DB migration, quorum ownership mutation, service activation, or rollback. Normal
process/network/logging effects and unrelated concurrent controllers are not
claimed absent. Python runtime bytecode/temp files are also outside this claim.

Fresh read-only B readback confirmed both retained archive digests, .30 current,
absent product transaction, failed historical units with empty ControlGroup, and
full stdout/stderr byte hashes matching incident.json. This narrows the recovery
case; absence of a product transaction by itself remains insufficient.
