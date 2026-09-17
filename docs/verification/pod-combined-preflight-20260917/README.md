# Combined fixture preflight failure

Disposable local fixture, 2026-09-17. Outcomes:[0,0,1]. Both real runtime workers
successfully started pinned PostgreSQL/local Redis using the authenticated module
from a test-signed bundle and fresh pinned Observer authorization callbacks.

Node1 schema preparation was blocked before creating a command container:
`ValueError: fixed internal bridge network required`.
The reviewed fixture network was:
`bb96c8c033c3fe346a44e7ad21d9d8acb7b702799b83d190c1d7639413c6270f`,
name siemcore-combined-peer, driver bridge, **Internal=false**.

Updates independently inspected the network, confirmed no stage container existed,
and queried node1: public schema table count0. The schema operation journal was
conservatively retained as incomplete/potential-partial. See responses.json for
runtime receipts and failure outcome; no credential material is included.

No runner protection was weakened. Docker cannot convert this network in place
through supported API. SiemCore will discard only the owned disposable fixture
and create a new internal-network fixture with new registry/installation identity.
That is a new qualification attempt, not a same-operation recovery claim.
Updates orchestrator/diagnostic containers exited with --rm; fresh name filters
showed none retained. Product owns exact dependency/network/volume cleanup after
collecting its evidence. No cloud VMs, live deployments or production data changed.
