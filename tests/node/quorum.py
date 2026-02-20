import pprint
import time

from .commands import check_if_node_is_running
from .actions import setup_rubix_nodes, create_and_register_did, \
    fund_did_with_rbt, quorum_config,add_peer_details
from config.utils import save_to_config_file, load_from_config_file

QUORUM_CONFIG_PATH = "./quorum_config.json"



def run_quorum_nodes(
        only_run_nodes, 
        skip_adding_quorums, 
        node_registry_key = "quorum", 
        quorum_config_path = "./quorum_config.json", 
        quorum_list_file_name = "quorumlist.json", 
        only_bip39 = False,
        concurrent = False
    ):
    print("Running Rubix Quorum nodes......")
    
    node_config = setup_rubix_nodes(node_registry_key, concurrent=concurrent)

    print("Rubix Quorum nodes are now running")

    did_alias = "did_quorum"
    did_type = 4
    node_did_alias_map = {}
    for node, config in node_config.items():
        if concurrent and not check_if_node_is_running(int(node.lstrip("node"))):
            raise Exception(f"{node} is NOT running. Exiting...")
        node_did_alias_map[node] = did_alias

    if not only_run_nodes:
        print("Creating, Registering and Funding Quorum DIDs\n")
        for node, config in node_config.items():
            did = create_and_register_did(config, did_alias, did_type, register_did=True)
            fund_did_with_rbt(config, did)


        save_to_config_file(quorum_config_path, node_config)
        print("\nquorum_config json file is created")
        
        print("Setting up quorums and saving the info in quorumlist.json")
        print("quorum list ext file name: ", quorum_list_file_name)
        quorum_config(node_config, node_did_alias_map, skip_adding_quorums, quorum_list_file_name, only_bip39)

        pprint.pp(node_config)
        print("Quorums have been configured")
    else:
        node_config = get_quorum_config()
        quorum_config(node_config, node_did_alias_map, True, quorum_list_file_name)

def get_quorum_config():
    return load_from_config_file(QUORUM_CONFIG_PATH)
