from node.actions import rbt_transfer, fund_did_with_rbt, setup_rubix_nodes, \
    create_and_register_did, add_quorums, add_peer_details
from node.utils import get_did_by_alias
from config.utils import save_to_config_file, load_from_config_file
from helper.utils import expect_failure, expect_success
from node.quorum import get_quorum_config

__node_config_path = "./periodic_pledging.json"

def setup():
    print("Setting up test.....")
    print("Configuring and running node15 and node16...")

    node_config = setup_rubix_nodes("periodic_pledging")

    config_A = node_config["node15"]
    config_B = node_config["node16"]

    create_and_register_did(config_A, "did_a", register_did=False)
    create_and_register_did(config_A, "did_a1", register_did=False)
    create_and_register_did(config_B, "did_b", register_did=False)

    save_to_config_file(__node_config_path, node_config)

    print("Adding quorums")
    add_quorums(node_config)

    print("Setup Done\n")
    return node_config

def run(skip_setup: bool = False):
    print("\n----------- 1. Running Tests related to Periodic Pledging -----------\n")
    node_config = {}

    # In some cases, we may wish to run tests for an existing test configuration
    # where the nodes are running already. If skip_setup is True, the setup steps
    # are skipped and we proceed to directly run the test cases and load the config
    # from the config file
    if not skip_setup:
        node_config = setup()
    else:
        node_config = load_from_config_file(__node_config_path)
        add_quorums(node_config)
    
    shuttle_transfer(node_config)

    print("\n-------------- Tests Completed -------------------\n")

def shuttle_transfer(config):
    node_A_info, node_B_info = config["node15"], config["node16"]
    server_port_A, grpc_port_A = node_A_info["server"], node_A_info["grpcPort"]
    server_port_B, grpc_port_B = node_B_info["server"], node_B_info["grpcPort"]
    did_A, did_B = get_did_by_alias(node_A_info, "did_a"), get_did_by_alias(node_B_info, "did_b")
     
    quorum_config = get_quorum_config()
    
    for _, val in quorum_config.items():
        add_peer_details(val["peerId"], val["dids"]["did_quorum"], 4, server_port_A, grpc_port_A)
        add_peer_details(val["peerId"], val["dids"]["did_quorum"], 4, server_port_B, grpc_port_B)

    print("------ Test Case (PASS): Shuttle transfer started ------\n")

    print("\n1. Generating 3 whole RBT for A")
    expect_success(fund_did_with_rbt)(node_A_info, did_A, 4)
    print("Funded node A with 4 RBT")

    print("\n2. Transferring 3 RBT from A to B....")
    add_peer_details(node_B_info["peerId"], did_B, 4, server_port_A, grpc_port_A)
    expect_success(rbt_transfer)(did_A, did_B, 3, server_port_A, grpc_port_A)
    print("Transferred 3 RBT from A to B")

    # print("Waiting for 80 seconds before ")
    # time.sleep(80)
    
    # print("\n3. Transferring 3 RBT from A to A1....")
    # expect_success(rbt_transfer)(address_B, address_A1, 3, server_port_B, grpc_port_B)
    # print("Transferred 3 RBT from A to A1")

    print("\n------ Test Case (PASS): Shuttle transfer completed ------\n")
