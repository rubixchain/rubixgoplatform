import os
import time
from node.actions import rbt_transfer, fund_did_with_rbt, setup_rubix_nodes, \
    create_and_register_did, add_quorums, add_peer_details, check_if_node_is_running, mint_ft, \
    transfer_ft, register_quorums_from_config
from node.utils import get_did_by_alias
from config.utils import save_to_config_file, load_from_config_file
from helper.utils import expect_failure, expect_success
from node.quorum import get_quorum_config, run_quorum_nodes
from node.commands import run_command
from prerequisite import get_os_info

__quorum_list_file_name = "quorumlist_rbt.json"
__quorum_config_file_name = "quorum_config_rbt.json"

def boot_quorums():
    run_quorum_nodes(
        False, 
        skip_adding_quorums=False,
        node_registry_key="quorum_rbt", 
        quorum_list_file_name=__quorum_list_file_name, 
        quorum_config_path=__quorum_config_file_name,
        only_bip39=True,
        concurrent=True
    )

def setup(is_mac_os=False):
    rbt_transfer_node_config = setup_rubix_nodes("nodes_rbt", concurrent=True)

    for node, config in rbt_transfer_node_config.items():
        if not check_if_node_is_running(int(node.lstrip("node"))):
            raise Exception(f"{node} is NOT running. Exiting...")
        print(f"{node} is running.")

    if is_mac_os:
        quorum_config = load_from_config_file(__quorum_config_file_name)
        for node, nq_config in rbt_transfer_node_config.items():
            for _, node_config in quorum_config.items():
                add_peer_details(
                    node_config["peerId"],
                    node_config["dids"]["did_quorum"]["did"],
                    nq_config["server"],
                    nq_config["grpcPort"]
                )

    create_and_register_did(rbt_transfer_node_config["node39"], "didA", register_did=True)
    create_and_register_did(rbt_transfer_node_config["node39"], "didA1", register_did=True)
    create_and_register_did(rbt_transfer_node_config["node40"], "didB", register_did=True)

    save_to_config_file("rbt_transfer_nodes_config.json", rbt_transfer_node_config)

    print("Adding quorum for node39")
    add_quorums(rbt_transfer_node_config, "node39", quorumlist=__quorum_list_file_name)

    print("Adding quorum for node40")
    add_quorums(rbt_transfer_node_config, "node40", quorumlist=__quorum_list_file_name)


def tests():
    # Re-register Quorum DIDs so that all the nodes have info about peerID of Quorum DIDs
    register_quorums_from_config(__quorum_config_file_name)

    rbt_transfer_node_config = load_from_config_file("rbt_transfer_nodes_config.json")

    didA = get_did_by_alias(rbt_transfer_node_config["node39"], "didA")
    didA1 = get_did_by_alias(rbt_transfer_node_config["node39"], "didA1")
    didB = get_did_by_alias(rbt_transfer_node_config["node40"], "didB")

    didA_port = rbt_transfer_node_config["node39"]["server"]
    didA_grpc = rbt_transfer_node_config["node39"]["grpcPort"]

    didB_port = rbt_transfer_node_config["node40"]["server"]
    didB_grpc = rbt_transfer_node_config["node40"]["grpcPort"]

    print("\n1. Generating 2 whole RBT for A")
    expect_success(fund_did_with_rbt)(rbt_transfer_node_config["node39"], didA, 2, 71)
    print("Funded node A with 2 RBT")

    print("\n2. Transferring 0.5 RBT from A to A1....")
    expect_success(rbt_transfer)(didA, didA1, 0.5, didA_port, didA_grpc)

    

    print("\n3. Transferring 1.499 RBT from A1 to A....")
    expect_success(rbt_transfer)(didA, didA1, 1.499, didA_port, didA_grpc)

    print("\n4. Transferring 0.25 RBT from A1 to A....")
    expect_success(rbt_transfer)(didA1, didA, 0.25, didA_port, didA_grpc)

    print("\n5. Transferring 0.25 RBT from A1 to A....")
    expect_success(rbt_transfer)(didA1, didA, 0.25, didA_port, didA_grpc)

    print("\n6. Transferring 0.25 RBT from A1 to A....")
    expect_success(rbt_transfer)(didA1, didA, 0.25, didA_port, didA_grpc)

    print("\n7. Transferring 0.25 RBT from A1 to A....")
    expect_success(rbt_transfer)(didA1, didA, 0.25, didA_port, didA_grpc)

    print("\n8. Transferring 1 RBT from A to A1....")
    expect_success(rbt_transfer)(didA, didA1, 1, didA_port, didA_grpc)
    print("Transfer Complete")



if __name__=='__main__':
    boot_quorums()

    _, os_name = get_os_info()

    if os_name == 'mac':
        setup(is_mac_os=True)
    else:
        setup(is_mac_os=False)

    tests()