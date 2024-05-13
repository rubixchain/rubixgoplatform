import pprint
import time

from .actions import setup_rubix_nodes, fetch_peer_ids, create_and_register_did, \
    fund_dids_with_rbt, quorum_config
from .utils import save_to_json

def run_quorum_nodes(node_config_path, only_run_nodes, skip_adding_quorums):
    node_config_path = "./quorum_config.json"
    
    print("Running Rubix nodes......")
    node_config = setup_rubix_nodes(5)
    print("Rubix nodes are now running")

    if not only_run_nodes:
        fetch_peer_ids(node_config)

        print("Creation and registeration of quorum DIDs have started")
        create_and_register_did(node_config)
        print("All quorum DIDs have been registered")

        print("Initiating funding of these quorum DIDs")
        fund_dids_with_rbt(node_config)
        print("All Quorum DIDs have been funded")
        
        save_to_json(node_config_path, node_config)
        
        print("Setting up quorums and saving information about them to quorumlist.json")
        quorum_config(node_config, skip_adding_quorums=skip_adding_quorums, create_quorum_list=True)

        pprint.pp(node_config)
        print("Quorums have been configured")
    else:
        quorum_config(node_config, skip_adding_quorums=True, create_quorum_list=False)

    return node_config

def run_non_quorum_nodes(node_config_path, only_run_nodes, skip_adding_quorums):
    node_config_path = "./non_quorum_config.json"

    print("Running non-quorum nodes...")
    node_config = setup_rubix_nodes(2, "nodeNq")
    print("Non-quorum nodes are running successfully")

    if not only_run_nodes:
        fetch_peer_ids(node_config)
        
        print("Creation of Non Quorum DIDs have started")
        create_and_register_did(node_config, False)
        print("Non Quorum DIDs have been created")

        save_to_json(node_config_path, node_config)

        print("Adding and setting up quorum config")
        quorum_config(node_config, skip_adding_quorums=skip_adding_quorums, create_quorum_list=False)

        pprint.pp(node_config)
        print("Non Quorum nodes have been configured")
    else:
        quorum_config(node_config, skip_adding_quorums=True, create_quorum_list=False)

    return node_config