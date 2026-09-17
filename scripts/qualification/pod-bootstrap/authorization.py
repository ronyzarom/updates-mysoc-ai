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


def validate_absence(sent, raw):
    ack=protocol.strict_json(raw)
    fields={'code','protocol','operation_id','registry_sha256','generation','node_id','input_sha256','processing_allowed'}
    if not isinstance(ack,dict) or set(ack)!=fields or ack['code']!='authorization_not_recorded':
        raise ValueError('unverified authorization absence')
    for field in ('protocol','operation_id','registry_sha256','input_sha256'):
        if ack[field]!=sent[field]:raise ValueError('absence binding mismatch')
    if (type(ack['generation']) is not int or ack['generation']<=0 or ack['generation']!=sent['generation'] or
            ack['node_id']!=sent['node']['node_id'] or ack['processing_allowed'] is not False):
        raise ValueError('absence identity or generation mismatch')
    return ack


def verify_invitation(invitation, pinned_key, expected, now_ns, require_current=True):
    """Validate detached exact-byte Ed25519 signature, scope and current lifetime."""
    import base64
    from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey
    if not isinstance(invitation, dict) or set(invitation) != {'payload_base64', 'signature'}:
        raise ValueError('exact invitation envelope required')
    if len(invitation['payload_base64']) > 5464 or len(invitation['signature']) > 88:
        raise ValueError('oversized invitation')
    raw=base64.b64decode(invitation['payload_base64'],validate=True)
    if len(raw)>4096:raise ValueError('oversized invitation payload')
    Ed25519PublicKey.from_public_bytes(pinned_key).verify(
        base64.b64decode(invitation['signature'],validate=True),b'mysoc-pod-bootstrap-invitation-v1\n'+raw)
    claims=protocol.strict_json(raw)
    fields={'protocol','authorization_id','operation_id','registry_sha256','node_id','input_sha256','issued_at','expires_at'}
    if not isinstance(claims,dict) or set(claims)!=fields:
        raise ValueError('exact invitation claims required')
    if claims['protocol']!='pod-bootstrap-invitation-v1' or not re.fullmatch(IDENTIFIER,claims['authorization_id']):
        raise ValueError('invalid invitation identity')
    for field in ('operation_id','registry_sha256','node_id','input_sha256'):
        if claims[field]!=expected[field]:raise ValueError('invitation binding mismatch')
    def nanos(value):
        seconds,fraction=timestamp(value)
        return int(seconds.timestamp())*1_000_000_000+fraction
    issued,expires=nanos(claims['issued_at']),nanos(claims['expires_at'])
    if not 0<expires-issued<=3600*1_000_000_000 or (require_current and not issued<=now_ns<expires):
        raise ValueError('invitation expired, future-dated or excessive lifetime')
    return claims


class Coordinator:
    """Authorization-only transport integration. Does not execute provisioning."""
    def __init__(self, registration, invitation, pinned_key, journal, wall_clock=None):
        import time
        self.registration=registration
        self.invitation=invitation
        self.pinned_key=pinned_key
        self.path=journal
        self.wall_clock=wall_clock or time.time_ns

    def run_locked(self, generation):
        import http.client
        import hashlib
        import json
        import client
        r=self.registration
        expected=dict(operation_id=r.registry['operation_id'],registry_sha256=r.fingerprint,
                      node_id=r.node_id,input_sha256=r.input_hash)
        existed=self.path.exists()
        claims=verify_invitation(self.invitation,self.pinned_key,expected,self.wall_clock(),require_current=not existed)
        binding=dict(expected,generation=generation,authorization_id=claims['authorization_id'],
                     expires_at=claims['expires_at'],authorization_key_sha256=hashlib.sha256(self.pinned_key).hexdigest())
        if existed:
            state=protocol.strict_json(client.protected(self.path))
            if set(state)!={'binding','phase'} or state['binding']!=binding or state['phase'] not in ('intent','authorized','authorization-expired'):
                raise ValueError('authorization change requires explicit reconciliation')
        else:
            client.durable(self.path,{'binding':binding,'phase':'intent'})
        def call(action):
            remaining=r.deadline-r.clock()
            if remaining<=0:raise TimeoutError('authorization deadline reached')
            if action=='authorize':
                verify_invitation(self.invitation,self.pinned_key,expected,self.wall_clock())
            sent=request(r.registry,r.fingerprint,r.node_id,action,generation,r.input_hash,
                         self.invitation if action=='authorize' else None)
            return validate_ack(sent,action,r.transport.call(action,sent,min(10,remaining)),
                                claims['authorization_id'],claims['expires_at'])
        def reconcile():
            try:return call('authorization-status')
            except client.AuthorizationNotRecorded as absent:
                sent=request(r.registry,r.fingerprint,r.node_id,'authorization-status',generation,r.input_hash)
                validate_absence(sent,absent.body)
                # call() rechecks current signature/scope/lifetime before this exact retry.
                return call('authorize')
        if existed:
            ack=reconcile()
        else:
            try:ack=call('authorize')
            except (OSError,http.client.HTTPException):ack=reconcile()
        client.durable(self.path,{'binding':binding,'phase':ack['phase']})
        return dict(ack,installation_complete=False)
