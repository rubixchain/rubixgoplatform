import pprint
import time

from .actions import setup_rubix_nodes, create_and_register_did, \
    fund_did_with_rbt, quorum_config
from config.utils import save_to_config_file, load_from_config_file

QUORUM_CONFIG_PATH = "./quorum_config.json"

def run_quorum_nodes(only_run_nodes, skip_adding_quorums):
    print("Running Rubix Quorum nodes......")
    node_registry_key = "quorum"
    node_config = setup_rubix_nodes(node_registry_key)
    print("Rubix Quorum nodes are now running")

    if not only_run_nodes:
        did_alias = "did_quorum"
        node_did_alias_map = {}

        print("Creating, Registering and Funding Quorum DIDs\n")
        for node, config in node_config.items():
            did = create_and_register_did(config, did_alias)

            fund_did_with_rbt(config, did)

            # Selecting DIDs for quorum setup
            node_did_alias_map[node] = did_alias
        save_to_config_file(QUORUM_CONFIG_PATH, node_config)
        print("\nquorum_config.json is created")
        
        print("Setting up quorums and saving the info in quorumlist.json")
        quorum_config(node_config, node_did_alias_map, skip_adding_quorums=skip_adding_quorums)

        pprint.pp(node_config)
        print("Quorums have been configured")
    else:
        quorum_config(node_config, node_did_alias_map, skip_adding_quorums=True)

def get_quorum_config():
    return load_from_config_file(QUORUM_CONFIG_PATH)
