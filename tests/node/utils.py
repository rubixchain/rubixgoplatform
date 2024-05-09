import json
import sys
import os
sys.path.insert(1, os.getcwd())
from .vars import QUORUM_NODES

def get_node_name_from_idx(idx, prefix_string: str = "node"):
    return prefix_string + str(idx)

def save_to_json(filepath, obj):
    # Check if file exists. If yes, then remove it
    if os.path.exists(filepath):
        os.remove(filepath)
    
    with open(filepath, 'w') as f:
        json.dump(obj, f, indent=4)
