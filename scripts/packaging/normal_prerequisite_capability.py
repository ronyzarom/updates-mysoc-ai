"""Read product hook capability declarations without executing product code."""
import ast
import hashlib
import re
from pathlib import Path

PROTOCOL = 'normal-prerequisites-v1'
DECLARATION = 'NORMAL_PREREQUISITES_PROTOCOL'


def marker_for_hook(hook, provisioning_commit):
    hook = Path(hook)
    if hook.is_symlink() or not hook.is_file():
        raise ValueError('regular provisioning hook required')
    raw = hook.read_bytes()
    tree = ast.parse(raw)
    declarations = []
    for statement in tree.body:
        if isinstance(statement, ast.Assign) and any(isinstance(target, ast.Name) and target.id == DECLARATION for target in statement.targets):
            declarations.append(ast.literal_eval(statement.value))
        elif isinstance(statement, ast.AnnAssign) and isinstance(statement.target, ast.Name) and statement.target.id == DECLARATION:
            declarations.append(ast.literal_eval(statement.value))
    if not declarations:
        return None  # Legacy kit remains available, without the new capability.
    if declarations != [PROTOCOL] or not any(isinstance(node, ast.FunctionDef) and node.name == 'require_normal_capability' for node in tree.body):
        raise ValueError('unsupported or incomplete Normal prerequisite hook declaration')
    if not re.fullmatch(r'[0-9a-f]{40}', provisioning_commit):
        raise ValueError('exact provisioning commit required')
    return dict(schema=1, protocol=PROTOCOL, provisioning_commit=provisioning_commit,
                hook_sha256=hashlib.sha256(raw).hexdigest())
