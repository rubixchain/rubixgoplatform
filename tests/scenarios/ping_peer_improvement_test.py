from node.actions import rbt_transfer, fund_did_with_rbt, setup_rubix_nodes, \
    create_and_register_did, add_quorums, add_peer_details
from node.utils import get_did_by_alias
from config.utils import save_to_config_file, load_from_config_file
from helper.utils import expect_failure, expect_success
from node.quorum import get_quorum_config, run_quorum_nodes

__node_config_path = "./ping_peer_config.json"

def setup():
    print("Setting up ping peer improvement test.....")
    print("Configuring and running node18 and node19...")

    node_config = setup_rubix_nodes("ping_peer")

    config_A = node_config["node18"]
    config_B = node_config["node19"]

    create_and_register_did(config_A, "did_a", register_did=False)
    create_and_register_did(config_B, "did_b", register_did=False)

    save_to_config_file(__node_config_path, node_config)

    run_quorum_nodes(False, False, "quorum2", "./quorum_config2.json", "quorumlist2.json")

    print("Adding quorums for A")
    add_quorums(node_config, "node18")

    print("Adding quorums for B")
    add_quorums(node_config, "node19", "quorumlist2.json")

    print("Setup Done\n")
    return node_config

def run(skip_setup: bool = False):
    print("\n----------- 1. Running Tests related to ping peer improvement -----------\n")
    node_config = {}

    # In some cases, we may wish to run tests for an existing test configuration
    # where the nodes are running already. If skip_setup is True, the setup steps
    # are skipped and we proceed to directly run the test cases and load the config
    # from the config file
    if not skip_setup:
        node_config = setup()
    else:
        node_config = load_from_config_file(__node_config_path)

    shuttle_transfer(node_config)

    print("\n-------------- Tests Completed -------------------\n")

def shuttle_transfer(config):
    node_A_info, node_B_info = config["node18"], config["node19"]
    server_port_A, grpc_port_A = node_A_info["server"], node_A_info["grpcPort"]
    server_port_B, grpc_port_B = node_B_info["server"], node_B_info["grpcPort"]
    did_A, did_B = get_did_by_alias(node_A_info, "did_a"), get_did_by_alias(node_B_info, "did_b")

    print("------ Test Case (PASS): Shuttle transfer started ------\n")

    quorum_config = get_quorum_config()
    
    for _, val in quorum_config.items():
        add_peer_details(val["peerId"], val["dids"]["did_quorum"], 4, server_port_A, grpc_port_A)

    quorum_config2 = load_from_config_file("./quorum_config2.json")
    for _, val in quorum_config2.items():
        add_peer_details(val["peerId"], val["dids"]["did_quorum"], 4, server_port_B, grpc_port_B)


    print("\n1. Generating 3 whole RBT for A")
    expect_success(fund_did_with_rbt)(node_A_info, did_A, 3)
    print("Funded node A with 2 RBT")

    print("\n2. Transferring 0.5 RBT from A to B....")
    add_peer_details(node_B_info["peerId"], did_B, 4, server_port_A, grpc_port_A) #adding peer details of node B to node A
    expect_success(rbt_transfer)(did_A, did_B, 0.5, server_port_A, grpc_port_A)
    print("Transferred 0.5 RBT from A to B")

    print("\n4. Transferring 0.25 RBT from B to A....")
    add_peer_details(node_A_info["peerId"], did_A, 4, server_port_B, grpc_port_B) #adding peer details of node A to node B
    expect_success(rbt_transfer)(did_B, did_A, 0.25, server_port_B, grpc_port_B)
    print("Transferred 0.25 RBT from B to A")

    print("\n3. Transferring 1.499 RBT from A to B....")
    expect_success(rbt_transfer)(did_A, did_B, 1.499, server_port_A, grpc_port_A)
    print("Transferred 1.499 RBT from A to B")
    
    print("\n5. Transferring 0.25 RBT from B to A....")
    expect_success(rbt_transfer)(did_B, did_A, 0.25, server_port_B, grpc_port_B)
    print("Transferred 0.25 RBT from B to A")

    print("\n6. Transferring 0.25 RBT from B to A....")
    expect_success(rbt_transfer)(did_B, did_A, 0.25, server_port_B, grpc_port_B)
    print("Transferred 0.25 RBT from B to A")

    print("\n8. Transferring 1 RBT from A to B....")
    expect_success(rbt_transfer)(did_A, did_B, 1, server_port_A, grpc_port_A)
    print("Transferred 1 RBT from A to B")    

    print("\n7. Transferring 0.25 RBT from B to A....")
    expect_success(rbt_transfer)(did_B, did_A, 0.25, server_port_B, grpc_port_B)
    print("Transferred 0.25 RBT from B to A")

    print("\n9. Generating 2 whole RBT for A")
    expect_success(fund_did_with_rbt)(node_A_info, did_A, 2)
    print("Funded node A with 2 RBT")
    
    print("\n10. Transferring 2 RBT from A to B....")
    expect_success(rbt_transfer)(did_A, did_B, 2, server_port_A, grpc_port_A)
    print("Transferred 2 RBT from A to B")    

    print("\n11. Transferring 0.001 RBT from A to B....")
    expect_success(rbt_transfer)(did_A, did_B, 0.001, server_port_A, grpc_port_A)
    print("Transferred 0.001 RBT from A to B")

    print("\n7. Transferring 1.25 RBT from B to A....")
    expect_success(rbt_transfer)(did_B, did_A, 1.25, server_port_B, grpc_port_B)
    print("Transferred 0.25 RBT from B to A")

    print("\n------ Test Case (PASS): Shuttle transfer completed ------\n")
