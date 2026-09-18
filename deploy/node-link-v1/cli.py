#!/usr/bin/env python3
"""Offline entry-point scaffold. Not an installable or advertised capability.

The product adoption contract and native qualification are pending. Deliberately
do not import the update executor, bootstrap hooks, or any host mutation code.
"""
import argparse
import json

PROTOCOL = "pod-node-link-v1"
ACTIONS = ("readiness", "apply", "status", "recover")


def dispatch(action):
    if action not in ACTIONS:
        raise ValueError("unsupported action")
    # Status/recover must not manufacture a successful or absent transaction:
    # no transaction store or product contract is qualified in this scaffold.
    return {
        "protocol": PROTOCOL,
        "action": action,
        "capability_enabled": False,
        "status": "unavailable",
        "error_code": "link_contract_not_qualified",
        "mutation": "none",
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=ACTIONS)
    args = parser.parse_args()
    print(json.dumps(dispatch(args.action), sort_keys=True))
    return 78  # Configuration/capability unavailable, never success.


if __name__ == "__main__":
    raise SystemExit(main())
