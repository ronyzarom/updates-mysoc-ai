"""Source-only authorization wire validation; no HTTP calls or stage execution."""
from datetime import datetime, timezone
import re
import protocol

IDENTIFIER = r'[A-Za-z0-9][A-Za-z0-9_-]{0,100}'
UTC_TIMESTAMP = r'([0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2})(?:\.([0-9]{1,9}))?Z'


def timestamp(value):
    match = re.fullmatch(UTC_TIMESTAMP, value) if isinstance(value, str) else None
    if not match:
        raise ValueError('UTC RFC3339Nano timestamp required')
    seconds = datetime.strptime(match[1], '%Y-%m-%dT%H:%M:%S').replace(tzinfo=timezone.utc)
    return seconds, int((match[2] or '').ljust(9, '0'))


def request(registry, fingerprint, node_id, action, generation, input_sha256, invitation=None):
    if action not in ('authorize', 'authorization-status'):
        raise ValueError('unsupported authorization action')
    sent = protocol.request(registry, fingerprint, node_id, 'register', generation, input_sha256)
    if action == 'authorize':
        if (not isinstance(invitation, dict) or set(invitation) != {'payload_base64', 'signature'} or
                not all(isinstance(v, str) and v for v in invitation.values())):
            raise ValueError('exact signed invitation envelope required')
        sent['invitation'] = dict(invitation)
    elif invitation is not None:
        raise ValueError('status must not submit an invitation')
    return sent


def validate_ack(sent, action, raw, intended_authorization_id, intended_expires_at):
    if action not in ('authorize', 'authorization-status'):
        raise ValueError('unsupported authorization action')
    if not isinstance(intended_authorization_id, str) or not re.fullmatch(IDENTIFIER, intended_authorization_id):
        raise ValueError('invalid intended authorization identity')
    expected_expiry = timestamp(intended_expires_at)
    ack = protocol.strict_json(raw)
    if not isinstance(ack, dict) or set(ack) != protocol.ACK_FIELDS | {'authorization_id', 'expires_at'}:
        raise ValueError('unexpected authorization fields')
    for field in ('protocol', 'operation_id', 'registry_sha256'):
        if ack[field] != sent[field]:
            raise ValueError('authorization binding mismatch')
    if (type(ack['generation']) is not int or ack['generation'] <= 0 or
            ack['generation'] != sent['generation'] or ack['processing_allowed'] is not False):
        raise ValueError('authorization generation or permission mismatch')
    phases = {'authorized'} if action == 'authorize' else {'authorized', 'authorization-expired'}
    if ack['phase'] not in phases:
        raise ValueError('unexpected authorization phase')
    if (ack['authorization_id'] != intended_authorization_id or
            timestamp(ack['expires_at']) != expected_expiry):
        raise ValueError('different authorization requires explicit reconciliation')
    # Even an authorized response is not an execution/activation permission.
    return ack
