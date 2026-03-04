import os
import time
from node.actions import rbt_transfer, fund_did_with_rbt, setup_rubix_nodes, \
    create_and_register_did, add_quorums, add_peer_details, check_if_node_is_running, mint_ft, \
    transfer_ft, register_quorums_from_config
from node.utils import get_did_by_alias
from config.utils import save_to_config_file, load_from_config_file
from helper.utils import expect_failure, expect_success, expect_success_within_retries
from node.quorum import get_quorum_config, run_quorum_nodes
from node.commands import run_command
from prerequisite import get_os_info

__quorum_list_file_name = "quorumlist_ft.json"
__quorum_config_file_name = "quorum_config_ft.json"


def boot_quorums():
    run_quorum_nodes(
        False, 
        skip_adding_quorums=False,
        node_registry_key="quorum_ft", 
        quorum_list_file_name=__quorum_list_file_name, 
        quorum_config_path=__quorum_config_file_name,
        only_bip39=True,
        concurrent=True
    )

def setup(is_mac_os=False):
    ft_transfer_node_config = setup_rubix_nodes("nodes_ft", concurrent=True)

    for node, config in ft_transfer_node_config.items():
        if not check_if_node_is_running(int(node.lstrip("node"))):
            raise Exception(f"{node} is NOT running. Exiting...")
        print(f"{node} is running.")

    if is_mac_os:
        quorum_config = load_from_config_file(__quorum_config_file_name)
        for node, nq_config in ft_transfer_node_config.items():
            for _, node_config in quorum_config.items():
                add_peer_details(
                    node_config["peerId"],
                    node_config["dids"]["did_quorum"]["did"],
                    nq_config["server"],
                    nq_config["grpcPort"]
                )

    create_and_register_did(ft_transfer_node_config["node11"], "didA", register_did=True)
    create_and_register_did(ft_transfer_node_config["node12"], "didB", register_did=True)

    save_to_config_file("ft_transfer_nodes_config.json", ft_transfer_node_config)

    print("Adding quorum for node11")
    add_quorums(ft_transfer_node_config, "node11", quorumlist=__quorum_list_file_name)

    print("Adding quorum for node12")
    add_quorums(ft_transfer_node_config, "node12", quorumlist=__quorum_list_file_name)

    # register all the quorum dids
    
def tests():
    # Re-register Quorum DIDs so that all the nodes have info about peerID of Quorum DIDs
    register_quorums_from_config(__quorum_config_file_name)

    ft_transfer_node_config = load_from_config_file("ft_transfer_nodes_config.json")

    # Mint 1000 FTs to DID of node4
    didA = get_did_by_alias(ft_transfer_node_config["node11"], "didA")
    didB = get_did_by_alias(ft_transfer_node_config["node12"], "didB")
    ft_creator_did_ABC = didA

    didA_port = ft_transfer_node_config["node11"]["server"]
    didB_port = ft_transfer_node_config["node12"]["server"]

    print("\n1. Generating 2 whole RBT for A1")
    expect_success(fund_did_with_rbt)(ft_transfer_node_config["node11"], didA, 11)
    print("Funded node A with 2 RBT")

    print("\n2. Minting 10000 ABC FTs to DID of node11 (didA)")
    expect_success(mint_ft)(ft_transfer_node_config["node11"], didA, "ABC", 10000, 10)

    print("\n3. Transferring 300 ABC FTs from DID of node11 (didA) to DID of node12 (didB)")
    expect_success_within_retries(transfer_ft)(didA, didB, ft_creator_did_ABC, "ABC", 300, didA_port)

    print("\n4. Transferring 300 ABC FTs from DID of node12 (didB) to DID of node11 (didA)")
    expect_success_within_retries(transfer_ft)(didB, didA, ft_creator_did_ABC, "ABC", 300, didB_port)

    print("\n5. Transferring 1000 ABC FTs from DID of node11 (didA) to DID of node12 (didB)")
    expect_success_within_retries(transfer_ft)(didA, didB, ft_creator_did_ABC, "ABC", 1000, didA_port)
    

    print("\n6. Transferring 100000 ABC FTs (inssufficient funds) from DID of node11 (didA) to DID of node12 (didB)")
    expect_failure(transfer_ft)(didA, didB, ft_creator_did_ABC, "ABC", 100000, didA_port)
    print("done")

if __name__=='__main__':
    boot_quorums()

    _, os_name = get_os_info()

    if os_name == 'mac':
        setup(is_mac_os=True)
    else:
        setup(is_mac_os=False)

    tests()