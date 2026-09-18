"""Protected source measurement. Caller must hold the shared lifecycle lock.

No writes, authority calls, credential output, or dependency downloads. Policy
pins the complete inventory of relevant historical JSON receipts. Unknown files
and any linked-operation history block admission, even without a link.json.
"""
import hashlib
import json
from pathlib import Path
import stat
import uuid
from protocol import strict_json
from transaction import digest

HISTORY_ROOTS = (
    '/var/lib/siemcore-node-update',
    '/var/lib/siemcore-bootstrap-repair',
    '/var/lib/siemcore-node-link',
    '/var/lib/siemcore-pod-maintenance',
)


class SourceLoader:
    def __init__(self, root='/', owner=0, expected_operation=None):
        self.root = Path(root)
        self.owner = owner  # fixture identity only; production constructs default.
        self.expected_operation = expected_operation

    def path(self, absolute):
        if not absolute.startswith('/') or '..' in Path(absolute).parts:
            raise ValueError('fixed_absolute_path_required')
        return self.root / absolute.lstrip('/')

    def protected(self, path):
        path = Path(path)
        for item in (path, *path.parents):
            info = item.lstat()
            if info.st_uid != self.owner or info.st_mode & 0o022 or stat.S_ISLNK(info.st_mode):
                raise ValueError('unprotected_source_path')
            if item != path and not stat.S_ISDIR(info.st_mode):
                raise ValueError('invalid_source_parent')
            if item == self.root:
                break
        return path

    def read(self, absolute):
        path = self.protected(self.path(absolute))
        if not path.is_file() or path.stat().st_size > 4*1024*1024:
            raise ValueError('bounded_source_file_required')
        return path.read_bytes()

    def inventory(self):
        found = {}
        for absolute in HISTORY_ROOTS:
            root = self.path(absolute)
            if not root.exists() and not root.is_symlink():
                continue
            self.protected(root)
            for item in sorted(root.rglob('*')):
                self.protected(item)
                if not item.is_file() and not item.is_dir():
                    raise ValueError('unexpected_history_type')
                if item.is_file() and item.suffix == '.json':
                    relative = '/' + item.relative_to(self.root).as_posix()
                    found[relative] = hashlib.sha256(self.read(relative)).hexdigest()
        return found

    def measure(self, expected_inventory):
        operations=self.path('/var/lib/siemcore-node-standalone/operations')
        if operations.exists() or operations.is_symlink():
            self.protected(operations)
            for operation in operations.iterdir():
                self.protected(operation)
                if not operation.is_dir() or operation.name!=self.expected_operation:
                    raise ValueError('prior_standalone_operation_requires_reconciliation')
        inventory = self.inventory()
        if inventory != expected_inventory:
            raise ValueError('historical_inventory_changed')
        if any(path.startswith(('/var/lib/siemcore-node-link/', '/var/lib/siemcore-pod-maintenance/')) for path in inventory):
            raise ValueError('linked_history_refused')
        app = strict_json(self.read('/etc/siemcore/greenfield.json'))
        identity = {key: app[key] for key in ('machine_id', 'installation_id', 'updater_instance_id', 'node_id')}
        if identity['machine_id'] != self.read('/etc/machine-id').decode().strip():
            raise ValueError('machine_identity_mismatch')
        if app.get('schema') != 5 or app.get('topology') != 'node-unlinked' or app.get('pod_id'):
            raise ValueError('independent_source_required')
        bootstrap = self.read('/var/lib/siemcore-greenfield/journal.json')
        release = strict_json(self.read('/etc/siemcore/greenfield-release.json'))
        current_version, current_sha = release['version'], release['sha256']
        records, updates = [], {}
        operation_prefix = '/var/lib/siemcore-node-update/operations/'
        for path in inventory:
            if path.startswith(operation_prefix):
                operation = path[len(operation_prefix):].split('/')[0]
                if operation_prefix + operation + '/adapter-journal.json' not in inventory:
                    raise ValueError('incomplete_historical_operation')
        for path in inventory:
            if path.endswith('/adapter-journal.json') and path.startswith('/var/lib/siemcore-node-update/operations/'):
                directory = str(Path(path).parent)
                operation = Path(directory).name
                if str(uuid.UUID(operation)) != operation:
                    raise ValueError('invalid_historical_operation')
                journal = strict_json(self.read(path))
                binding = strict_json(self.read(directory + '/binding.json'))
                if journal.get('protocol') != 'pod-node-update-v1' or journal.get('operation_id') != operation or journal.get('operation_sha256') != digest(binding) or binding.get('operation_id') != operation:
                    raise ValueError('historical_binding_mismatch')
                if any(binding.get(k) != v for k, v in identity.items()) or journal.get('phase') != 'accepted':
                    raise ValueError('unreconciled_update_history')
                updates[operation] = binding
                records.append(dict(protocol='pod-node-update-v1', phase='accepted', identity=identity, receipt_sha256=inventory[path]))
            elif path.endswith('/receipt.json') and path.startswith('/var/lib/siemcore-bootstrap-repair/'):
                receipt = strict_json(self.read(path))
                if receipt.get('phase') != 'journal_committed':
                    raise ValueError('unreconciled_repair_history')
                # Full raw receipt is already pinned by reviewed policy inventory.
                records.append(dict(protocol='node-bootstrap-repair', phase='journal_committed', identity=identity, receipt_sha256=inventory[path]))
        active = '/var/lib/siemcore-node-update/active-operation.json'
        if updates or active in inventory:
            selected = strict_json(self.read(active))['operation_id']
            if selected not in updates:
                raise ValueError('active_update_not_accepted')
            # Reconcile ancestry all the way back to the original signed release.
            seen = set()
            while True:
                if selected in seen or selected not in updates:
                    raise ValueError('invalid_update_ancestry')
                seen.add(selected)
                binding = updates[selected]
                if len(seen) == 1:
                    current_version, current_sha = binding['target']['version'], binding['target']['artifact_sha256']
                previous = binding['predecessor']
                ancestor_path = '/var/lib/siemcore-node-update/operations/' + selected + '/previous-operation.json'
                if ancestor_path not in inventory:
                    if (previous['version'], previous['artifact_sha256'], previous['artifact_signature']) != (release['version'], release['sha256'], release['signature']):
                        raise ValueError('bootstrap_ancestry_mismatch')
                    break
                selected = strict_json(self.read(ancestor_path))['operation_id']
                if selected not in updates or updates[selected]['target'] != previous:
                    raise ValueError('update_ancestry_mismatch')
            if seen != set(updates):
                raise ValueError('unreconciled_update_branch')
        linked = []
        for path in ('/opt/siemcore-node-unlinked-'+identity['node_id']+'/link.json', '/etc/siemcore-pod-node/link.json', '/etc/siemcore-pod-controller/controller.json', '/var/lib/siemcore-greenfield/pod-runtime.json'):
            if self.path(path).exists() or self.path(path).is_symlink():
                linked.append(path)
        evidence = dict(application=app, journals=records, current_version=current_version,
                        current_artifact_sha256=current_sha, linked_evidence=linked, inventory_complete=True)
        return evidence, bootstrap, release
