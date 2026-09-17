"""Validate real-product fixture provenance. No service startup or mutation here."""
import hashlib
from pathlib import Path

MODES = {'readiness', 'maintenance-v1', 'recovery-v2', 'next-operation', 'drain-discovery', 'drain-recovery'}


def argv(value):
    return isinstance(value, list) and bool(value) and all(isinstance(x, str) and x for x in value) and Path(value[0]).is_absolute()


def validate_plan(plan):
    if plan.get('adapter_kind') != 'product':
        return
    if not plan.get('isolated') or plan.get('evidence_tier') not in ('native-with-synthetic-host', 'executable-real-host'):
        raise ValueError('product fixtures require explicit isolated native evidence tier')
    pins = plan.get('product_binaries')
    if not isinstance(pins, dict) or not pins:
        raise ValueError('product binary SHA256 pins required')
    verify_pins(pins)
    if not plan.get('scenarios'):
        raise ValueError('product scenarios required')
    for scenario in plan['scenarios']:
        for command in ('reset_command', 'ready_command', 'verify_command', 'stop_command'):
            if not argv(scenario.get(command)):
                raise ValueError('product fixture requires absolute ' + command)
        if scenario.get('launch_command') and not argv(scenario['launch_command']):
            raise ValueError('launch command must use absolute argv')
        if not scenario.get('runs'):
            raise ValueError('scenario runs required')
        for step in scenario['runs']:
            if step.get('expected') not in ('success', 'failure') or not Path(step.get('config', '')).is_absolute():
                raise ValueError('absolute case config and expected result required')


def verify_pins(pins):
    for name, expected in pins.items():
        path = Path(name)
        if not path.is_absolute() or path.is_symlink() or not path.is_file():
            raise ValueError('product binary must be an absolute regular file')
        actual = hashlib.sha256(path.read_bytes()).hexdigest()
        if actual != expected:
            raise ValueError('product binary changed: ' + name)


def validate_case(plan, case, proxy=None):
    if plan.get('adapter_kind') != 'product':
        return {}
    verify_pins(plan['product_binaries'])
    if case.get('mode') not in MODES or not Path(case.get('directory', '')).is_absolute():
        raise ValueError('supported coordinator mode and absolute journal directory required')
    command = case.get('adapter_command')
    if proxy is not None:
        if not proxy.get('isolated'):
            raise ValueError('isolated proxy required')
        command = proxy.get('adapter_command')
    if not argv(command):
        raise ValueError('product adapter argv required')
    if any(Path(part).name == 'reference_adapter.py' for part in command):
        raise ValueError('reference responder cannot be used as product evidence')
    used = {part: plan['product_binaries'][part] for part in command if part in plan['product_binaries']}
    if not used:
        raise ValueError('adapter argv must invoke a pinned product executable')
    return used
