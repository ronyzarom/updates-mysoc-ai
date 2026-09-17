"""Immutable schema-4 data-copy direction; never a processing-role assignment."""
import hashlib
import protocol


def validate(original_input,registry,input_sha256,node_id,seed_config):
    if hashlib.sha256(original_input).hexdigest()!=input_sha256:
        raise ValueError('original input bytes changed')
    document=protocol.strict_json(original_input)
    app=document['application']
    if type(app.get('schema')) is not int or app['schema']!=4 or app.get('topology')!='pod':
        raise ValueError('schema4 pod input required')
    if app.get('cluster_id')!=registry['pod_id']:
        raise ValueError('original pod identity mismatch')
    sync=app.get('selective_sync')
    if not isinstance(sync,dict) or set(sync)!={'source_node_id','target_node_id','allowlist_version'}:
        raise ValueError('exact application.selective_sync required')
    source,target=sync['source_node_id'],sync['target_node_id']
    data_nodes={n['node_id'] for n in registry['nodes'] if n['node_id'] in ('1','2')}
    if source not in data_nodes or target not in data_nodes or source==target or data_nodes!={'1','2'}:
        raise ValueError('distinct registered data-copy nodes required')
    if type(sync['allowlist_version']) is not int or sync['allowlist_version']!=2:
        raise ValueError('unsupported selective-copy allowlist version')
    # This qualification implements only the explicitly agreed clean-rebuild direction.
    if (source,target)!=('1','2'):raise ValueError('unqualified initial seed direction')
    if seed_config is not None:
        if node_id!=target or seed_config.get('seed',{}).get('source_node_id')!=source:
            raise ValueError('seed configuration contradicts immutable original input')
    return dict(sync)
