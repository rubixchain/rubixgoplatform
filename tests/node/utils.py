import json
import sys
import os
sys.path.insert(1, os.getcwd())
from .vars import QUORUM_NODES

def get_node_name_from_idx(idx, prefix_string: str = "node"):
    return prefix_string + str(idx)

def get_base_ports():
    base_ens_server = 20000
    base_grpc_port = 10500

    return base_ens_server, base_grpc_port

def get_did_by_alias(node_config, alias):
    return node_config["dids"][alias]["did"]