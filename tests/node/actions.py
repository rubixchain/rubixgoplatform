import time

from typing import List
from .api import api_get_smart_contract_chain, SmartContractTokenChainReq, \
    SCDataReply, api_get_nft_chain, NFTDataReply, NFTTokenChainReq, api_create_nft
from .commands import check_if_node_is_running, cmd_deploy_smart_contract, cmd_execute_smart_contract, cmd_generate_smart_contract, cmd_run_rubix_servers, cmd_get_peer_id, cmd_create_did, cmd_register_did, \
    cmd_generate_rbt, cmd_add_quorum_dids, cmd_setup_quorum_dids, cmd_rbt_transfer, cmd_subscribe_smart_contract, get_build_dir, cmd_add_peer_details, \
    cmd_mint_ft, cmd_transfer_ft, cmd_create_nft, cmd_deploy_nft, cmd_self_execute_nft, cmd_transfer_nft, cmd_subscribe_nft
from .utils import get_node_name_from_idx, get_did_by_alias
from config.utils import load_from_config_file, save_to_config_file, get_node_registry

def register_quorums_from_config(config_path: str):
    quorum_config = load_from_config_file(config_path)
    for _, config in quorum_config.items():
        quorum_did = config["dids"]["did_quorum"]["did"]
        cmd_register_did(quorum_did, config["server"], config["grpcPort"])   

def add_quorums(node_config: dict, node_key = "", quorumlist = "quorumlist.json"):
    if node_key == "":
        for config in node_config.values():
            cmd_add_quorum_dids(
                config["server"], 
                config["grpcPort"]
            )
    else: 
        config = node_config[node_key]
        cmd_add_quorum_dids(
            config["server"], 
            config["grpcPort"],
            quorumlist
        )

def setup_quorums(node_config: dict, node_did_alias_map: dict, simple_setup: bool = False):
    for node, config in node_config.items():
        did = get_did_by_alias(config, node_did_alias_map[node])
        if node in {"node4", "node5"} and not simple_setup:
            priv_pwd="p123"
            quorum_pwd="q123"
        else:
            priv_pwd = "mypassword" 
            quorum_pwd = "mypassword"
        cmd_setup_quorum_dids(
            did,
            config["server"],
            config["grpcPort"],
            priv_pwd,
            quorum_pwd
        )

def quorum_config(node_config: dict, node_did_alias_map: dict, skip_adding_quorums: bool = False, quorum_list_file_name = "quorumlist.json", simple_setup: bool = False):
    # Prepare quorumlist.json
    quorum_list = []
    build_dir = get_build_dir()
    quorum_list_file_path = f"../{build_dir}/{quorum_list_file_name}"
 
    if skip_adding_quorums:
        setup_quorums(node_config, node_did_alias_map)
    else:
        for node, config in node_config.items():
            did = get_did_by_alias(config, node_did_alias_map[node])
            quorum_info = {
                "type": 2,
                "address": did
            }
            
            quorum_list.append(quorum_info)

        print("internal value for quorum var: ", quorum_list_file_path)
        save_to_config_file(quorum_list_file_path, quorum_list)

        # add_quorums(node_config)

        setup_quorums(node_config, node_did_alias_map, simple_setup)


def setup_rubix_nodes_concurrent(node_registry_config_key):
    return setup_rubix_nodes(node_registry_config_key, concurrent=True)

def setup_rubix_nodes(node_registry_config_key, concurrent=False):
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
        node_server, grpc_server = cmd_run_rubix_servers(node_name, idx, concurrent=concurrent)

        cfg = {
            "dids": {},
            "server": node_server,
            "grpcPort": grpc_server,
            "peerId": "",
        }

        if not concurrent:
            fetch_peer_id(cfg)
        
        node_config[node_name] = cfg

    retry_count = 2
    retry_attempt = 1
    if concurrent:
        print("Waiting for nodes to start to get their peer IDs...")
        time.sleep(100)  
        for idx in node_indices:
            node_name = get_node_name_from_idx(idx)
            if not check_if_node_is_running(int(node_name.lstrip("node"))):
                if retry_attempt <= retry_count:
                    print(f"{node_name} is NOT running. Retrying {retry_attempt}/{retry_count} after 30s...")
                    time.sleep(30)
                    if not check_if_node_is_running(int(node_name.lstrip("node"))):
                        raise Exception(f"{node_name} is NOT running. Exiting...")
                    retry_attempt += 1
                
            fetch_peer_id(node_config[node_name])

    return node_config

def fetch_peer_id(config):
    peer_id = cmd_get_peer_id(config["server"], config["grpcPort"])
    config["peerId"] = peer_id

def create_and_register_did(config: dict, did_alias: str, did_type: int = 4, register_did: bool = True, fp: bool = False):
    if fp:
        print(f"creating did with fp flag")
        did = cmd_create_did(config["server"], config["grpcPort"], did_type, "p123", "q123")
        print(f"DID {did} has been created successfully")
        config["dids"][did_alias] = {}
        config["dids"][did_alias]["did"] = did
        config["dids"][did_alias]["did_type"] = did_type

        if register_did:
            cmd_register_did(did, config["server"], config["grpcPort"], "p123")
            print(f"DID {did} has been registered successfully")

        return did
    else:
        did = cmd_create_did(config["server"], config["grpcPort"], did_type)
        print(f"DID {did} has been created successfully")

        config["dids"][did_alias] = {}
        config["dids"][did_alias]["did"] = did
        config["dids"][did_alias]["did_type"] = did_type

        if register_did:
            cmd_register_did(did, config["server"], config["grpcPort"])
            print(f"DID {did} has been registered successfully")

        return did

def fund_did_with_rbt(node_config: dict, did: str,  rbt_amount: int = 70, priv_pwd="mypassword"):
    cmd_generate_rbt(did, rbt_amount, node_config["server"], node_config["grpcPort"], priv_pwd)
    print("DID ", did, f" is funded with {rbt_amount} RBT")

def rbt_transfer(
        sender_address: str, 
        receiver_address: str, 
        transfer_rbt: float, 
        sender_server_port: int, 
        sender_grpc_port: int,
        priv_pwd="mypassword"):
    cmd_rbt_transfer(sender_address, receiver_address, transfer_rbt, sender_server_port, sender_grpc_port, priv_pwd)

def add_peer_details(peer_id: str, did_id: str, did_type: int, server_port: int, grpc_port: int):
    cmd_add_peer_details(peer_id, did_id, did_type, server_port, grpc_port)


## FT

def mint_ft(node_config: dict, did: str, ftName: str, ftAmount: int, amount: int, priv_pwd="mypassword"):
    cmd_mint_ft(did, ftAmount, ftName, amount, node_config["server"], priv_pwd)
    print(f"FT minted successfully to DID {did} with amount {amount}")

def transfer_ft(sender_address: str, receiver_address: str, creatorDid: str,  ft_name: str, ft_count: int, sender_server_port: int, priv_pwd="mypassword"):
    cmd_transfer_ft(sender_address, receiver_address, creatorDid, ft_name, ft_count, sender_server_port, priv_pwd)
    print(f"FT {ft_name} transferred successfully from {sender_address} to {receiver_address} with count {ft_count}")


# Smart Contract

def generate_smart_contract(did, raw_code, bin_code, server_port, priv_pwd="mypassword"):
    smart_contract_hash = ""

    smart_contract_hash = cmd_generate_smart_contract(
        did,
        raw_code,
        bin_code,
        server_port,
        priv_pwd=priv_pwd
    )

    if smart_contract_hash == "":
        raise Exception("Smart Contract generation failed")

    return smart_contract_hash


def deploy_smart_contract(smart_contract_id: str, did: str, rbtAmount: float, server_port, priv_pwd="mypassword"):
    cmd_deploy_smart_contract(
        smart_contract_id,
        did,
        rbtAmount,
        server_port,
        priv_pwd=priv_pwd
    )

def execute_smart_contract(smart_contract_id: str, did: str, server_port, priv_pwd="mypassword", sctData="test"):
    cmd_execute_smart_contract(
        smart_contract_id,
        did,
        server_port,
        priv_pwd=priv_pwd,
        sctData="test"
    )

def subscribe_smart_contract(smart_contract_id: str, did: str, server_port, priv_pwd="mypassword"):
    cmd_subscribe_smart_contract(
        smart_contract_id,
        did,
        server_port,
        priv_pwd=priv_pwd
    )

def get_smart_contract_chain(smart_contract_hash: str, server_port: str, latest_block: bool = False) -> List[SCDataReply]:
    request = SmartContractTokenChainReq(token=smart_contract_hash, latest=latest_block)

    resp = api_get_smart_contract_chain(req=request, server_port=server_port)
    if not resp["status"]:
        raise Exception("failed to fetch smart contract token chain, err: ", resp["message"])
    else:        
        return resp["SCTDataReply"]


# NFT

def generate_nft(did, artifact_path, metadata_path, server_port, priv_pwd="mypassword"):
    nft_id = ""

    nft_id = api_create_nft(
        artifact_path,
        metadata_path,
        did,
        server_port,
    )

    if nft_id == "":
        raise Exception("Smart Contract generation failed")

    return nft_id


def deploy_nft(nft_id: str, did: str, rbtAmount: float, server_port, priv_pwd="mypassword"):
    cmd_deploy_nft(
        did,
        nft_id,
        rbtAmount,
        server_port
    )

def self_execute_nft(nft_id: str, did: str, nft_value: float, server_port, priv_pwd="mypassword", nftData="test"):
    cmd_self_execute_nft(
        did,
        nft_id,
        nft_value,
        server_port
    )

def transfer_nft(nft_id: str, sender_did: str, receiver_did: str, nft_value: float, server_port, priv_pwd="mypassword", sctData="test"):
    cmd_transfer_nft(
        executor_did=sender_did,
        receiver_did=receiver_did,
        nft_id=nft_id,
        nft_value=nft_value,
        server_port=server_port
    )

def subscribe_nft(nft_id: str, server_port, priv_pwd="mypassword"):
    cmd_subscribe_nft(
        nft_id,
        server_port,
    )

def get_nft_chain(nft_id: str, server_port: str, latest_block: bool = False) -> List[NFTDataReply]:
    request = NFTTokenChainReq(nft=nft_id, latest=latest_block)

    resp = api_get_nft_chain(req=request, server_port=server_port)

    if not resp["status"]:
        raise Exception("failed to fetch smart contract token chain, err: ", resp["message"])
    else:        
        return resp["NFTDataReply"]

