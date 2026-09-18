"""Pure offline admission checks; not signature verification or authorization.

protected_binding must eventually come from a root-owned verified release and
adoption policy, never from the transport request. No executor calls this module
until that policy loader and independently authenticated barrier are qualified.
"""
import hashlib
from urllib.parse import urlsplit

from product_contract import parse_request


def validate_candidate(raw, protected_binding, adoption_plan_bytes):
    # Product parser caps characters; enforce the transport's byte limit too.
    if not isinstance(raw, bytes) or len(raw) > 32768:
        raise ValueError("invalid_request_size")
    request = parse_request(raw)
    binding = request["binding"]
    if binding != protected_binding:
        raise ValueError("protected_binding_mismatch")
    if not isinstance(adoption_plan_bytes, bytes):
        raise ValueError("invalid_adoption_plan")
    if hashlib.sha256(adoption_plan_bytes).hexdigest() != binding["adoption_plan_sha256"]:
        raise ValueError("adoption_plan_digest_mismatch")
    # Equivalent HTTPS origins must not evade product fixed-endpoint separation
    # through hostname case, a trailing DNS dot, or explicit default port.
    def origin(value):
        parsed = urlsplit(value)
        return parsed.hostname.lower().rstrip("."), parsed.port or 443
    if origin(binding["observer"]["endpoint"]) == origin(binding["pod"]["customer_url"]):
        raise ValueError("observer_must_use_fixed_endpoint")
    return request
