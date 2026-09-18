# Standalone node transition — offline admission

`transaction.py` validates a separate exact operation binding and a protected
source-evidence snapshot, including original independent schema/identity,
completed original bootstrap receipt, current artifact, and prior root journal
inventory. Unfinished, unknown or linked history blocks admission. Evidence must
match a reviewed policy digest; caller assertions or absence of a link file are
not authorization. There are no Observer calls.

`persist_intent` stores the source and requested target mode separately in an
atomic fsynced journal with exclusive locking and conflicting replay refusal.
The effective mode remains `independent-management`; routine updates stay false.
There is no function to write launch mode.json or mark standalone accepted.

This is **not installed or executable on hosts**. Remaining root integration must
load protected policy/keys, enumerate and reconcile all real journal directories
under the lifecycle lock, verify source journal ancestry/current signed artifact,
verify target archive signatures and measured manifest/binary, enforce data/AI/
MySoc/TLS preflight, stage/switch/recover the product and validate full health.
The inventory snapshots used in tests are fixtures, not proof about a live host.
The root directory/ancestor chain must be protected before using these libraries.

Receipt records normalized by the future measurement adapter use protocol, phase,
immutable identity and raw receipt SHA256; actual journals must be read and
verified before producing those records. Do not skip unknown paths or substitute
`inventory_complete=true` for enumeration. Existing bootstrap history is read-only.

Run `python3 -m unittest scripts.tests.test_standalone_configuration
scripts.tests.test_standalone_transaction` (11 fixture tests). Native product
qualification and routine standalone update execution remain pending. Normal,
Observer and existing management-only update paths are unchanged.
