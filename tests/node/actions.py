from .commands import cmd_run_rubix_servers, cmd_get_peer_id, cmd_create_did, cmd_register_did, \
    cmd_generate_rbt, cmd_add_quorum_dids, cmd_setup_quorum_dids, cmd_rbt_transfer, get_build_dir
from .utils import get_node_name_from_idx, save_to_json

def add_quorums(node_config: dict):
    for config in node_config.values():
        cmd_add_quorum_dids(
            config["server"], 
            config["grpcPort"]
        )

def setup_quorums(node_config: dict):
    for config in node_config.values():
        cmd_setup_quorum_dids(
            config["did"],
            config["server"],
            config["grpcPort"]
        )

def quorum_config(node_config: dict, skip_adding_quorums: bool = False, create_quorum_list: bool = False):
    # Prepare quorumlist.json
    quorum_list = []
    if create_quorum_list:
        for config in node_config.values():
            quorum_info = {
                "type": 2,
                "address": config["peerId"] + "." + config["did"]
            }
            
            build_dir = get_build_dir()
            quorum_list_file_path = f"../{build_dir}/quorumlist.json" 
            quorum_list.append(quorum_info)

        save_to_json(quorum_list_file_path, quorum_list)

    # # add quorums
    if not skip_adding_quorums:
        add_quorums(node_config)

    setup_quorums(node_config)


def get_base_ports():
    base_ens_server = 20000
    base_grpc_port = 10500

    return base_ens_server, base_grpc_port

def setup_rubix_nodes(node_count: int = 0, node_prefix_str: str = "node"):
    base_ens_server, base_grpc_port = get_base_ports()
    
    node_config = {}

    # Start rubix servers
    loop_start_idx, loop_end_idx = 0, node_count 
    offset = 4

    for i in range(loop_start_idx, loop_end_idx):
        k = (i + offset) if node_prefix_str == "node" else (10 + i + offset)

        ens_server = base_ens_server + k
        print(f"Running server at port: {ens_server}")
        grpc_port = base_grpc_port + k

        node_name = get_node_name_from_idx(k, node_prefix_str)
        
        cmd_run_rubix_servers(node_name, k, grpc_port)
        
        node_config[node_name] = {
            "did": "",
            "server": ens_server,
            "grpcPort": grpc_port,
            "peerId": ""
        }

    return node_config


def fetch_peer_ids(node_config: dict):
    print("Fetching Node IDs........")
    for config in node_config.values():
        peer_id = cmd_get_peer_id(config["server"], config["grpcPort"])
        config["peerId"] = peer_id
    print("Fetched all Node IDs")


def create_and_register_did(node_config: dict, register_did: bool = True):
    for config in node_config.values():
        did_id = cmd_create_did(config["server"], config["grpcPort"])
        print("Created DID : ", did_id)
        
        if register_did:
            print(f"Registering DID: {did_id}")
            cmd_register_did(did_id, config["server"], config["grpcPort"])
            print("DID is registered successfully\n")

        config["did"] = did_id

def fund_dids_with_rbt(node_config: dict, rbt_amount: int = 30):
    for node, config in node_config.items():
        if node not in ["node5", "node6", "node7", "node8"]:
            cmd_generate_rbt(config["did"], rbt_amount, config["server"], config["grpcPort"])
            print("DID ", config["did"], f" is funded with {rbt_amount} RBT")

def fund_did_with_rbt(config: dict, rbt_amount: int = 30):
    output = cmd_generate_rbt(config["did"], rbt_amount, config["server"], config["grpcPort"])
    print(output)
    return output

def rbt_transfer(config_sender: dict, config_receiver: dict, transfer_rbt_amount: int):
    sender_address = config_sender["peerId"] + "." + config_sender["did"]
    receiver_address = config_receiver["peerId"] + "." + config_receiver["did"]

    cmd_rbt_transfer(sender_address, receiver_address, transfer_rbt_amount, config_sender["server"], config_sender["grpcPort"])