"""Worker transport vocabulary for standalone enablement, never POD authority."""
from datetime import datetime, timezone
import json
from transaction import PROTOCOL, digest, validate_binding


def strict_json(raw):
    def pairs(items):
        result = {}
        for key, value in items:
            if key in result:
                raise ValueError('duplicate_json_field')
            result[key] = value
        return result
    if len(raw) > 65536:
        raise ValueError('json_limit')
    return json.loads(raw, object_pairs_hook=pairs,
                      parse_constant=lambda _: (_ for _ in ()).throw(ValueError('nonfinite_json')))


def validate_response(response, binding):
    validate_binding(binding)
    required = {'protocol', 'operation_id', 'operation_sha256', 'phase', 'observed_at', 'mutation'}
    if not isinstance(response, dict) or not required <= set(response) or set(response) - required - {'health', 'error_code'}:
        raise ValueError('invalid_worker_response')
    if response['protocol'] != PROTOCOL or response['operation_id'] != binding['operation_id'] or response['operation_sha256'] != digest(binding):
        raise ValueError('worker_binding_mismatch')
    if response['phase'] not in {'prepared', 'staged', 'switching', 'verifying', 'accepted', 'recovery_required', 'restoring', 'restored', 'blocked'} or response['mutation'] not in {'none', 'possible', 'confirmed'}:
        raise ValueError('invalid_worker_phase')
    observed = datetime.fromisoformat(response['observed_at'].replace('Z', '+00:00'))
    if observed.tzinfo is None or not -5 <= (datetime.now(timezone.utc) - observed).total_seconds() <= 30:
        raise ValueError('stale_worker_response')
    return response
