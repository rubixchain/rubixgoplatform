from node.actions import rbt_transfer, fund_did_with_rbt, setup_rubix_nodes, \
    create_and_register_did, add_quorums
from node.utils import get_did_by_alias
from config.utils import save_to_config_file, load_from_config_file
from helper.utils import expect_failure, expect_success

__node_config_path = "./bip39_nlss_config.json"

def setup():
    print("Setting up test.....")
    print("Configuring and running node11 and node12...")

    node_config = setup_rubix_nodes("bip39_nlss")

    config_bip39 = node_config["node11"]
    config_nlss = node_config["node12"]

    create_and_register_did(config_bip39, "bip39_1", did_type=4)


    create_and_register_did(config_nlss, "nlss_1", did_type=0)

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
    address_bip39, address_nlss = node_bip39["peerId"]+"."+did_bip39, node_nlss["peerId"]+"."+did_nlss
    
    print("------ Test Case (PASS): Transferring whole, part and mix RBT from NLSS DID to BIP39 DID ------\n")

    print("\n1. Generating 3 RBT for NLSS DID")
    expect_success(fund_did_with_rbt)(node_nlss, did_nlss, 3)
    print("Funded NLSS DID with 3 RBT")

    print("\n2. Transferring 1 RBT from NLSS DID to BIP39 DID....")
    expect_success(rbt_transfer)(address_nlss, address_bip39, 1, server_port_nlss, grpc_port_nlss)
    print("Transferred 1 RBT from NLSS DID to BIP39 DID")

    print("\n3. Transferring 1.5 RBT from NLSS DID to BIP39 DID....")
    expect_success(rbt_transfer)(address_nlss, address_bip39, 1.5, server_port_nlss, grpc_port_nlss)
    print("Transferred 1.5 RBT from NLSS DID to BIP39 DID")

    print("\n4. Transferring 0.5 RBT from NLSS DID to BIP39 DID....")
    expect_success(rbt_transfer)(address_nlss, address_bip39, 0.5, server_port_nlss, grpc_port_nlss)
    print("Transferred 0.5 RBT from NLSS DID to BIP39 DID")

    print("\n------ Test Case (PASS): Transferring whole, part and mix RBT from NLSS DID to BIP39 DID completed ------\n")

def bip39_to_nlss(node_config):
    node_bip39, node_nlss = node_config["node11"], node_config["node12"]
    server_port_bip39, grpc_port_bip39 = node_bip39["server"], node_bip39["grpcPort"]
    did_bip39, did_nlss = get_did_by_alias(node_bip39, "bip39_1"), get_did_by_alias(node_nlss, "nlss_1")
    address_bip39, address_nlss = node_bip39["peerId"]+"."+did_bip39, node_nlss["peerId"]+"."+did_nlss
    
    print("------ Test Case (PASS): Transferring whole, part and mix RBT from BIP39 DID to NLSS DID ------\n")

    print("\n4. Transferring 0.5 RBT from BIP39 DID to NLSS DID....")
    expect_success(rbt_transfer)(address_bip39, address_nlss, 0.5, server_port_bip39, grpc_port_bip39)
    print("Transferred 0.5 RBT from BIP39 DID to NLSS DID")

    print("\n3. Transferring 1.5 RBT from BIP39 DID to NLSS DID....")
    expect_success(rbt_transfer)(address_bip39, address_nlss, 1.5, server_port_bip39, grpc_port_bip39)
    print("Transferred 1.5 RBT from BIP39 DID to NLSS DID")

    print("\n2. Transferring 1 RBT from BIP39 DID to NLSS DID....")
    expect_success(rbt_transfer)(address_bip39, address_nlss, 1, server_port_bip39, grpc_port_bip39)
    print("Transferred 1 RBT from BIP39 DID to NLSS DID")

