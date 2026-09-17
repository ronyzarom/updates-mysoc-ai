"""Isolated wire-contract helper. No network, installer, activation or VM actions."""
import json
import re

PROTOCOL = 'pod-bootstrap-v1'
ACTIONS = {'begin-or-resume': 'barrier', 'status': 'barrier',
           'register': 'registered', 'all-registered': 'all-registered'}
ACK_FIELDS = {'protocol', 'operation_id', 'registry_sha256', 'generation',
              'phase', 'processing_allowed'}

def strict_json(raw):
    def pairs(items):
        result = {}
        for key, value in items:
            if key in result:
                raise ValueError('duplicate JSON field')
            result[key] = value
        return result
    return json.loads(raw, object_pairs_hook=pairs)

def request(registry, fingerprint, node_id, action, generation=0, input_sha256=None):
    if action not in ACTIONS or type(generation) is not int or generation < 0:
        raise ValueError('unsupported action/generation')
    if action == 'begin-or-resume' and generation != 0:
        raise ValueError('begin requires generation zero')
    if action in ('register', 'all-registered') and generation <= 0:
        raise ValueError('persisted server generation required')
    if action == 'register':
        if not isinstance(input_sha256, str) or not re.fullmatch('[0-9a-f]{64}', input_sha256):
            raise ValueError('original input digest required')
    elif input_sha256 is not None:
        raise ValueError('input digest only for register')
    nodes = [n for n in registry['nodes'] if n['node_id'] == node_id]
    if len(nodes) != 1:
        raise ValueError('exact registered node required')
    result = dict(protocol=PROTOCOL, operation_id=registry['operation_id'],
                  registry_sha256=fingerprint, generation=generation, node=dict(nodes[0]))
    if input_sha256 is not None:
        result['input_sha256'] = input_sha256
    return result

def validate_ack(sent, action, raw):
    ack = strict_json(raw)
    if not isinstance(ack, dict) or set(ack) != ACK_FIELDS:
        raise ValueError('unexpected response fields')
    for field in ('protocol', 'operation_id', 'registry_sha256'):
        if ack[field] != sent[field]:
            raise ValueError('response binding mismatch')
    if type(ack['generation']) is not int or ack['generation'] <= 0:
        raise ValueError('positive server generation required')
    if sent['generation'] and ack['generation'] != sent['generation']:
        raise ValueError('server generation changed')
    if ack['phase'] != ACTIONS[action] or ack['processing_allowed'] is not False:
        raise ValueError('response cannot grant execution or processing')
    return ack
