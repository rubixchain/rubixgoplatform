from .commands import cmd_run_rubix_servers, cmd_get_peer_id, cmd_create_did, cmd_register_did, \
    cmd_generate_rbt, cmd_add_quorum_dids, cmd_setup_quorum_dids, cmd_rbt_transfer, get_build_dir
from .utils import get_node_name_from_idx, get_did_by_alias
from config.utils import save_to_config_file, get_node_registry

def add_quorums(node_config: dict):
    for config in node_config.values():
        cmd_add_quorum_dids(
            config["server"], 
            config["grpcPort"]
        )

def setup_quorums(node_config: dict, node_did_alias_map: dict):
    for node, config in node_config.items():
        did = get_did_by_alias(config, node_did_alias_map[node])
        cmd_setup_quorum_dids(
            did,
            config["server"],
            config["grpcPort"]
        )

def quorum_config(node_config: dict, node_did_alias_map: dict, skip_adding_quorums: bool = False):
    # Prepare quorumlist.json
    quorum_list = []
    build_dir = get_build_dir()
    quorum_list_file_path = f"../{build_dir}/quorumlist.json"
 
    if skip_adding_quorums:
        setup_quorums(node_config, node_did_alias_map)
    else:
        for node, config in node_config.items():
            did = get_did_by_alias(config, node_did_alias_map[node])
            quorum_info = {
                "type": 2,
                "address": config["peerId"] + "." + did
            }
            
            quorum_list.append(quorum_info)

        save_to_config_file(quorum_list_file_path, quorum_list)

        add_quorums(node_config)

        setup_quorums(node_config, node_did_alias_map)


def setup_rubix_nodes(node_registry_config_key):
    if node_registry_config_key == "":
        raise Exception("a key is needed to fetch node_registry.json config")
    
    node_registry = get_node_registry()
    if not node_registry_config_key in node_registry:
        raise Exception(f"config key {node_registry_config_key} not found in node_registry.json config")

    node_indices = node_registry[node_registry_config_key]

    if not isinstance(node_indices, list):
        raise Exception(f"the correspoding value for {node_registry_config_key} in node_registry.json must of List type")

    if len(node_indices) == 0:
        raise Exception(f"no indices found for {node_registry_config_key} in node_registry.json, provide at least one index")

    node_config = {}

    for idx in node_indices:
        node_name = "node" + str(idx)
        node_server, grpc_server = cmd_run_rubix_servers(node_name, idx)

        cfg = {
            "dids": {},
            "server": node_server,
            "grpcPort": grpc_server,
            "peerId": ""
        }

        fetch_peer_id(cfg)
        node_config[node_name] = cfg

    return node_config

def fetch_peer_id(config):
    peer_id = cmd_get_peer_id(config["server"], config["grpcPort"])
    config["peerId"] = peer_id

def create_and_register_did(config: dict, did_alias: str, did_type: int = 4, register_did: bool = True):
    did = cmd_create_did(config["server"], config["grpcPort"], did_type)
    print(f"DID {did} has been created successfully")

    config["dids"] = {
        did_alias: did
    }

    if register_did:
        cmd_register_did(did, config["server"], config["grpcPort"])
        print(f"DID {did} has been registered successfully")

    return did

def fund_did_with_rbt(node_config: dict, did: str,  rbt_amount: int = 70):
    cmd_generate_rbt(did, rbt_amount, node_config["server"], node_config["grpcPort"])
    print("DID ", did, f" is funded with {rbt_amount} RBT")

def rbt_transfer(
        sender_address: str, 
        receiver_address: str, 
        transfer_rbt: float, 
        sender_server_port: int, 
        sender_grpc_port: int):
    cmd_rbt_transfer(sender_address, receiver_address, transfer_rbt, sender_server_port, sender_grpc_port)
