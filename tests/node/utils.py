import json
import random
import sys
import os
import re
from datetime import datetime, timezone

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

def extract_cid_v0_from_msg(msg: str) -> str:
    pattern = r'\bQm[1-9A-HJ-NP-Za-km-z]{44}\b'

    cids = re.findall(pattern, msg)

    if len(cids) == 0:
        raise ValueError("Error: No CID v0 found in the text")
    elif len(cids) > 1:
        raise ValueError(f"Error: Multiple CIDs found ({len(cids)} CIDs). Expected exactly 1 CID")
    else:
        return cids[0]

def update_file_with_random_seed(file_path):
    current_utc = datetime.now(timezone.utc)
    utc_time_str = current_utc.strftime("%Y-%m-%d %H:%M:%S UTC")
    
    # Generate random seed
    random_seed = random.randint(100000, 999999)
    
    # Create the comment line
    header_line = f"// {utc_time_str} - {random_seed}\n"
    
    # Write to the file
    with open(file_path, 'w') as file:
        file.write(header_line)
    
    return random_seed

def update_sample_rust_file(file_path):
    update_file_with_random_seed(file_path)

def update_sample_artifact_file(file_path):
    update_file_with_random_seed(file_path)