#!/usr/bin/env python3
"""Truthful capability probe; not installed or wired to updater or product UI."""
import argparse
import json


def response(action):
    if action not in ('readiness', 'apply', 'status', 'recover'):
        raise ValueError('unsupported_action')
    return dict(protocol='pod-adoption-v1', action=action, capability_enabled=False,
                adoption_required=True, status='unavailable', operation_state='unknown',
                error_code='host_adoption_not_qualified',
                reasons=['protected_identity_loader_not_qualified',
                         'product_host_adoption_not_qualified',
                         'membership_executor_routing_not_qualified'],
                processing_authorized=False, mutation='none')


if __name__ == '__main__':
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('action', choices=('readiness', 'apply', 'status', 'recover'))
    print(json.dumps(response(p.parse_args().action), sort_keys=True))
    raise SystemExit(78)
