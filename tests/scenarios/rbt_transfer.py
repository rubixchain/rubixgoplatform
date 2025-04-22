from node.actions import rbt_transfer, fund_did_with_rbt, setup_rubix_nodes, \
    create_and_register_did, add_quorums, add_peer_details
from node.utils import get_did_by_alias
from config.utils import save_to_config_file, load_from_config_file
from helper.utils import expect_failure, expect_success
from node.quorum import get_quorum_config, run_quorum_nodes

__node_config_path = "./rbt_transfer_config.json"

def setup():
    print("Setting up test.....")
    print("Configuring and running node9 and node10...")

    node_config = setup_rubix_nodes("rbt_transfer")

    config_A = node_config["node9"]
    config_B = node_config["node10"]
    
    config_quorum = get_quorum_config()
    config_quorum_node4 = config_quorum["node4"]
    config_quorum_node5 = config_quorum["node5"]

    create_and_register_did(config_A, "did_a", register_did=True)
    
    # Sender and Receiver on same Non-Quorum server Scenario
    create_and_register_did(config_A, "did_a1", register_did=True)
    create_and_register_did(config_A, "did_a2", register_did=True)

    # Sender and Receiver on same Quorum server Scenario
    create_and_register_did(config_quorum_node4, "did_quorum_a1_node4", register_did=True)
    create_and_register_did(config_quorum_node4, "did_quorum_a2_node4", register_did=True)
    create_and_register_did(config_quorum_node5, "did_quorum_a1_node5", register_did=True)
    create_and_register_did(config_A, "did_nonquorum_a1_node9", register_did=True)

    create_and_register_did(config_B, "did_b", register_did=True)

    save_to_config_file(__node_config_path, node_config)
    save_to_config_file("./quorum_config.json", config_quorum)

    run_quorum_nodes(False, False, "quorum2", "./quorum_config2.json", "quorumlist2.json")

    print("Adding quorums for A")
    add_quorums(node_config, "node9")

    print("Adding quorums for B")
    add_quorums(node_config, "node10", "quorumlist2.json")

    print("Setup Done\n")
    return node_config

def run(skip_setup: bool = False):
    print("\n----------- 1. Running Tests related to RBT Transfer -----------\n")
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
    did_on_same_server_transfer(node_config)
    did_on_same_quorum_server_transfer(node_config)
    insufficient_balance_transfer(node_config)
    max_decimal_place_transfer(node_config)

    print("\n-------------- Tests Completed -------------------\n")

def did_on_same_server_transfer(config):
    node_A_info = config["node9"]

    server_port_A, grpc_port_A = node_A_info["server"], node_A_info["grpcPort"]
    did_A1, did_A2 = get_did_by_alias(node_A_info, "did_a1"), get_did_by_alias(node_A_info, "did_a2")

    print("------ Test Case (PASS): Transfer between DID's on same server ------\n")
  
    print("\n1. Generating 2 whole RBT for A1")
    expect_success(fund_did_with_rbt)(node_A_info, did_A1, 2)
    print("Funded node A with 2 RBT")

    print("\n2. Transferring 0.5 RBT from A1 to A2....")
    expect_success(rbt_transfer)(did_A1, did_A2, 0.5, server_port_A, grpc_port_A)
    print("Transfer Complete")

    print("\n3. Transferring 1.499 RBT from A1 to A2....")
    expect_success(rbt_transfer)(did_A1, did_A2, 1.499, server_port_A, grpc_port_A)
    print("Transfer Complete")

    print("\n4. Transferring 0.25 RBT from A2 to A1....")
    expect_success(rbt_transfer)(did_A2, did_A1, 0.25, server_port_A, grpc_port_A)
    print("Transfer Complete")

    print("\n5. Transferring 0.25 RBT from A2 to A1....")
    expect_success(rbt_transfer)(did_A2, did_A1, 0.25, server_port_A, grpc_port_A)
    print("Transfer Complete")

    print("\n6. Transferring 0.25 RBT from A2 to A1....")
    expect_success(rbt_transfer)(did_A2, did_A1, 0.25, server_port_A, grpc_port_A)
    print("Transfer Complete")

    print("\n7. Transferring 0.25 RBT from A2 to A1....")
    expect_success(rbt_transfer)(did_A2, did_A1, 0.25, server_port_A, grpc_port_A)
    print("Transfer Complete")

    print("\n8. Transferring 1 RBT from A1 to A2....")
    expect_success(rbt_transfer)(did_A1, did_A2, 1, server_port_A, grpc_port_A)
    print("Transfer Complete")    

    print("\n9. Generating 2 whole RBT for A1")
    expect_success(fund_did_with_rbt)(node_A_info, did_A1, 2)
    print("Transfer Complete")
    
    print("\n10. Transferring 2 RBT from A1 to A2....")
    expect_success(rbt_transfer)(did_A1, did_A2, 2, server_port_A, grpc_port_A)
    print("Transfer Complete")

    print("\n11. Transferring 0.001 RBT from A1 to A2....")
    expect_success(rbt_transfer)(did_A1, did_A2, 0.001, server_port_A, grpc_port_A)
    print("Transfer Complete")

    print("\n------ Test Case (PASS): Transfer between DID's on same server completed ------\n")

def did_on_same_quorum_server_transfer(config):
    quorum_config = get_quorum_config()

    node_4_info = quorum_config["node4"]
    node_5_info = quorum_config["node5"]
    node_9_info = config["node9"]

    server_port_4, grpc_port_4 = node_4_info["server"], node_4_info["grpcPort"]
    did_4_A, did_4_B = get_did_by_alias(node_4_info, "did_quorum_a1_node4"), get_did_by_alias(node_4_info, "did_quorum_a2_node4")

    server_port_5, grpc_port_5 = node_5_info["server"], node_5_info["grpcPort"]
    did_5_A = get_did_by_alias(node_5_info, "did_quorum_a1_node5")

    server_port_9, grpc_port_9 = node_9_info["server"], node_9_info["grpcPort"]
    did_9_A = get_did_by_alias(node_9_info, "did_nonquorum_a1_node9")

    for _, val in quorum_config.items():
        add_peer_details(val["peerId"], val["dids"]["did_quorum"]["did"], 4, server_port_4, grpc_port_4)
        add_peer_details(val["peerId"], val["dids"]["did_quorum"]["did"], 4, server_port_5, grpc_port_5)
        add_peer_details(val["peerId"], val["dids"]["did_quorum"]["did"], 4, server_port_9, grpc_port_9)
    
    print("------ Test Case (PASS): Transfer between DID's where either the Sender or Receiver are on a Quorum node ------\n")
  
    print("\n1. Generating 2 whole RBT for A on node4 (node4 is a Quorum node)")
    expect_success(fund_did_with_rbt)(node_4_info, did_4_A, 2)
    print("Funded node A with 2 RBT")

    print("\n2. Transferring 1 RBT from A to B on node4 (node4 is a Quorum node)")
    expect_success(rbt_transfer)(did_4_A, did_4_B, 1, server_port_4, grpc_port_4)
    print("Transferred 1 RBT from A to B")

    print("\n3. Transferring 1 RBT from B to A on node4 (node4 is a Quorum node)")
    expect_success(rbt_transfer)(did_4_B, did_4_A, 1, server_port_4, grpc_port_4)
    print("Transferred 1 RBT from A to B")

    print("\n4. Transferring 0.555 RBT from A to B on node4 (node4 is a Quorum node)")
    expect_success(rbt_transfer)(did_4_A, did_4_B, 0.555, server_port_4, grpc_port_4)
    print("Transferred 1 RBT from A to B")

    print("\n5. Transferring 0.555 RBT from B to A on node4 (node4 is a Quorum node)")
    expect_success(rbt_transfer)(did_4_B, did_4_B, 0.555, server_port_4, grpc_port_4)
    print("Transferred 1 RBT from A to B")

    print("\n6. Transferring 0.445 RBT from A of node4 to A of node5 (node4 and node5 are Quorum nodes)")
    add_peer_details(node_5_info["peerId"], did_5_A, 4, server_port_4, grpc_port_4)
    expect_success(rbt_transfer)(did_4_A, did_5_A, 0.445, server_port_4, grpc_port_4)
    print("Transferred 1 RBT from A to B")

    print("\n7. Transferring 0.445 RBT from A of node5 to A of node4 (node4 and node5 are Quorum nodes)")
    add_peer_details(node_4_info["peerId"], did_4_A, 4, server_port_5, grpc_port_5)
    expect_success(rbt_transfer)(did_5_A, did_4_A, 0.445, server_port_5, grpc_port_5)
    print("Transferred 1 RBT from A to B")

    print("\n8. Transferring 1 RBT from A of node4 to A of node9 (node4 is a Quorum node, node9 is a Non-Quorum node)")
    add_peer_details(node_9_info["peerId"], did_9_A, 4, server_port_4, grpc_port_4)
    expect_success(rbt_transfer)(did_4_A, did_9_A, 1, server_port_4, grpc_port_4)
    print("Transferred 1 RBT from A to B")

    print("\n9. Transferring 1 RBT from A of node9 to A of node4 (node4 is a Quorum node, node9 is a Non-Quorum node)")
    add_peer_details(node_4_info["peerId"], did_4_A, 4, server_port_9, grpc_port_9)
    expect_success(rbt_transfer)(did_9_A, did_4_A, 1, server_port_9, grpc_port_9)
    print("Transferred 1 RBT from A to B")

    print("------ Test Case (PASS): Transfer between DID's where either the Sender or Receiver are on a Quorum node completed------\n")

def max_decimal_place_transfer(config):
    node_A_info, node_B_info = config["node9"], config["node10"]
    server_port_B, grpc_port_B = node_B_info["server"], node_B_info["grpcPort"]
    did_A, did_B = get_did_by_alias(node_A_info, "did_a"), get_did_by_alias(node_B_info, "did_b")

    print("------ Test Case (FAIL) : Transferring 0.00000009 RBT from B which is more than allowed decimal places ------")

    print("\nTransferring 0.00000009 RBT from B to A....")
    expect_failure(rbt_transfer)(did_B, did_A, 0.00000009, server_port_B, grpc_port_B)

    print("\n------ Test Case (FAIL) : Transferring 0.00000009 RBT from B which is more than allowed decimal places completed ------\n")

def insufficient_balance_transfer(config):
    node_A_info, node_B_info = config["node9"], config["node10"]
    server_port_A, grpc_port_A = node_A_info["server"], node_A_info["grpcPort"]
    server_port_B, grpc_port_B = node_B_info["server"], node_B_info["grpcPort"]
    did_A, did_B = get_did_by_alias(node_A_info, "did_a"), get_did_by_alias(node_B_info, "did_b")

    print("\n------ Test Case (FAIL) : Transferring 100 RBT from A which has zero balance ------")

    print("\nTransferring 100 RBT from A to B....")
    expect_failure(rbt_transfer)(did_A, did_B, 100, server_port_A, grpc_port_A)

    print("\n------ Test Case (FAIL) : Transferring 100 RBT from A which has zero balance completed ------\n")

    print("\n------ Test Case (FAIL) : Transferring 100 RBT from B which has insufficient balance ------")

    print("\nTransferring 100 RBT from B to A....")
    expect_failure(rbt_transfer)(did_B, did_A, 0.25, server_port_B, grpc_port_B)

    print("\n------ Test Case (FAIL) : Transferring 100 RBT from B which has insufficient balance completed ------\n")

def shuttle_transfer(config):
    node_A_info, node_B_info = config["node9"], config["node10"]
    server_port_A, grpc_port_A = node_A_info["server"], node_A_info["grpcPort"]
    server_port_B, grpc_port_B = node_B_info["server"], node_B_info["grpcPort"]
    did_A, did_B = get_did_by_alias(node_A_info, "did_a"), get_did_by_alias(node_B_info, "did_b")

    print("------ Test Case (PASS): Shuttle transfer started ------\n")

    quorum_config = get_quorum_config()
    
    for _, val in quorum_config.items():
        add_peer_details(val["peerId"], val["dids"]["did_quorum"]["did"], val["dids"]["did_quorum"]["did_type"], server_port_A, grpc_port_A)

    quorum_config2 = load_from_config_file("./quorum_config2.json")
    for _, val in quorum_config2.items():
        add_peer_details(val["peerId"], val["dids"]["did_quorum"]["did"], val["dids"]["did_quorum"]["did_type"], server_port_B, grpc_port_B)

    print("\n1. Generating 2 whole RBT for A")
    expect_success(fund_did_with_rbt)(node_A_info, did_A, 2)
    print("Funded node A with 2 RBT")

    print("\n2. Transferring 0.5 RBT from A to B....")
    add_peer_details(node_B_info["peerId"], did_B, 4, server_port_A, grpc_port_A) #adding peer details of node B to node A
    expect_success(rbt_transfer)(did_A, did_B, 0.5, server_port_A, grpc_port_A)
    print("Transferred 0.5 RBT from A to B")

    print("\n3. Transferring 1.499 RBT from A to B....")
    expect_success(rbt_transfer)(did_A, did_B, 1.499, server_port_A, grpc_port_A)
    print("Transferred 1.499 RBT from A to B")

    print("\n4. Transferring 0.25 RBT from B to A....")
    add_peer_details(node_A_info["peerId"], did_A, 4, server_port_B, grpc_port_B) #adding peer details of node A to node B
    expect_success(rbt_transfer)(did_B, did_A, 0.25, server_port_B, grpc_port_B)
    print("Transferred 0.25 RBT from B to A")

    print("\n5. Transferring 0.25 RBT from B to A....")
    expect_success(rbt_transfer)(did_B, did_A, 0.25, server_port_B, grpc_port_B)
    print("Transferred 0.25 RBT from B to A")

    print("\n6. Transferring 0.25 RBT from B to A....")
    expect_success(rbt_transfer)(did_B, did_A, 0.25, server_port_B, grpc_port_B)
    print("Transferred 0.25 RBT from B to A")

    print("\n7. Transferring 0.25 RBT from B to A....")
    expect_success(rbt_transfer)(did_B, did_A, 0.25, server_port_B, grpc_port_B)
    print("Transferred 0.25 RBT from B to A")

    print("\n8. Transferring 1 RBT from A to B....")
    expect_success(rbt_transfer)(did_A, did_B, 1, server_port_A, grpc_port_A)
    print("Transferred 1 RBT from A to B")    

    print("\n9. Generating 2 whole RBT for A")
    expect_success(fund_did_with_rbt)(node_A_info, did_A, 2)
    print("Funded node A with 2 RBT")
    
    print("\n10. Transferring 2 RBT from A to B....")
    expect_success(rbt_transfer)(did_A, did_B, 2, server_port_A, grpc_port_A)
    print("Transferred 2 RBT from A to B")    

    print("\n11. Transferring 0.001 RBT from A to B....")
    expect_success(rbt_transfer)(did_A, did_B, 0.001, server_port_A, grpc_port_A)
    print("Transferred 0.001 RBT from A to B")

    print("\n------ Test Case (PASS): Shuttle transfer completed ------\n")
