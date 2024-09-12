from node.actions import rbt_transfer, fund_did_with_rbt, setup_rubix_nodes, \
    create_and_register_did, add_quorums, add_peer_details
from node.utils import get_did_by_alias
from config.utils import save_to_config_file, load_from_config_file
from helper.utils import expect_failure, expect_success
from node.quorum import get_quorum_config

__node_config_path = "./bip39_nlss_config.json"

def setup():
    print("Setting up test.....")
    print("Configuring and running node11 and node12...")

    node_config = setup_rubix_nodes("bip39_nlss")

    config_bip39 = node_config["node11"]
    config_nlss = node_config["node12"]

    # create_and_register_did(config_bip39, "bip39_1", did_type=4, register_did=False)
    create_and_register_did(config_bip39, "bip39_1", register_did=False, fp=True)


    # create_and_register_did(config_nlss, "nlss_1", did_type=0, register_did=False)
    create_and_register_did(config_nlss, "nlss_1", register_did=False, fp=True)

    save_to_config_file(__node_config_path, node_config)

    print("Adding quorums")
    add_quorums(node_config)

    print("Setup Done\n")
    return node_config

def run(skip_setup: bool = False):
    print("\n----------- 2. Running Tests related to RBT Transfer between BIP39 and NLSS dids -----------\n")
    node_config = {}

    if not skip_setup:
        node_config = setup()
    else:
        node_config = load_from_config_file(__node_config_path)
    
    nlss_to_bip39(node_config)
    bip39_to_nlss(node_config)
    print("\n-------------- Tests Completed -------------------\n")

def nlss_to_bip39(node_config):
    node_bip39, node_nlss = node_config["node11"], node_config["node12"]
    server_port_nlss, grpc_port_nlss = node_nlss["server"], node_nlss["grpcPort"]
    did_bip39, did_nlss = get_did_by_alias(node_bip39, "bip39_1"), get_did_by_alias(node_nlss, "nlss_1")
    
    print("------ Test Case (PASS): Transferring whole, part and mix RBT from NLSS DID to BIP39 DID ------\n")

    print("\n1. Generating 3 RBT for NLSS DID")
    expect_success(fund_did_with_rbt)(node_nlss, did_nlss, 3)
    print("Funded NLSS DID with 3 RBT")

    quorum_config = get_quorum_config()
    
    for _, val in quorum_config.items():
        add_peer_details(val["peerId"], val["dids"]["did_quorum"], val["did_type"], server_port_nlss, grpc_port_nlss)

    print("\n2. Transferring 1 RBT from NLSS DID to BIP39 DID....")
    add_peer_details(node_bip39["peerId"], did_bip39, 4, server_port_nlss, grpc_port_nlss) #adding peer details of bip39 node to nlss
    expect_success(rbt_transfer)(did_nlss, did_bip39, 1, server_port_nlss, grpc_port_nlss, "p123")
    print("Transferred 1 RBT from NLSS DID to BIP39 DID")

    print("\n3. Transferring 1.5 RBT from NLSS DID to BIP39 DID....")
    expect_success(rbt_transfer)(did_nlss, did_bip39, 1.5, server_port_nlss, grpc_port_nlss, "p123")
    print("Transferred 1.5 RBT from NLSS DID to BIP39 DID")

    print("\n4. Transferring 0.5 RBT from NLSS DID to BIP39 DID....")
    expect_success(rbt_transfer)(did_nlss, did_bip39, 0.5, server_port_nlss, grpc_port_nlss, "p123")
    print("Transferred 0.5 RBT from NLSS DID to BIP39 DID")

    print("\n------ Test Case (PASS): Transferring whole, part and mix RBT from NLSS DID to BIP39 DID completed ------\n")

def bip39_to_nlss(node_config):
    node_bip39, node_nlss = node_config["node11"], node_config["node12"]
    server_port_bip39, grpc_port_bip39 = node_bip39["server"], node_bip39["grpcPort"]
    did_bip39, did_nlss = get_did_by_alias(node_bip39, "bip39_1"), get_did_by_alias(node_nlss, "nlss_1")

    quorum_config = get_quorum_config()
    
    for _, val in quorum_config.items():
        add_peer_details(val["peerId"], val["dids"]["did_quorum"], val["did_type"], server_port_bip39, grpc_port_bip39)
    
    print("------ Test Case (PASS): Transferring whole, part and mix RBT from BIP39 DID to NLSS DID ------\n")

    print("\n4. Transferring 0.5 RBT from BIP39 DID to NLSS DID....")
    expect_success(rbt_transfer)(did_bip39, did_nlss, 0.5, server_port_bip39, grpc_port_bip39, "p123")
    print("Transferred 0.5 RBT from BIP39 DID to NLSS DID")

    print("\n3. Transferring 1.5 RBT from BIP39 DID to NLSS DID....")
    expect_success(rbt_transfer)(did_bip39, did_nlss, 1.5, server_port_bip39, grpc_port_bip39, "p123")
    print("Transferred 1.5 RBT from BIP39 DID to NLSS DID")

    print("\n2. Transferring 1 RBT from BIP39 DID to NLSS DID....")
    expect_success(rbt_transfer)(did_bip39, did_nlss, 1, server_port_bip39, grpc_port_bip39, "p123")
    print("Transferred 1 RBT from BIP39 DID to NLSS DID")

