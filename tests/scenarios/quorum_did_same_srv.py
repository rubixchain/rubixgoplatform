from node.actions import rbt_transfer, fund_did_with_rbt, setup_rubix_nodes, \
    create_and_register_did, add_quorums, setup_quorum, add_peer_details
from node.utils import get_did_by_alias
from node.commands import get_build_dir
from config.utils import save_to_config_file, load_from_config_file
from helper.utils import expect_failure, expect_success
from node.quorum import get_quorum_config, run_quorum_nodes

__node_config_path = "./multiple_quorum_on_same_srv.json"

def run_nodes():
    print("Running Rubix Quorum nodes..")
    node_config = setup_rubix_nodes("same_quorum")
    print("Rubix Quorum nodes are now running")

    return node_config

def node_config_all_same_func(node_config):
    build_dir = get_build_dir()
    quorum_list_file = f"../{build_dir}/quorumlist_all_same_node.json"

    node_config_19 = node_config["node19"]
    node_config_20 = node_config["node20"]
    node_config_21 = node_config["node21"]

    # Create Quorum DIDs
    quorum_did_arr = []
    for i in range(1, 8):
        did = create_and_register_did(node_config_19, f"did_q_{i}", register_did=False)
        quorum_did_arr.append(did)

        fund_did_with_rbt(node_config_19, did, 20)

    # Create quorumlist config file
    quorum_list = []
    for did in quorum_did_arr:
        quorum_info = {
            "type": 2,
            "address": did
        }
        
        quorum_list.append(quorum_info)

    save_to_config_file(quorum_list_file, quorum_list)
    
    # Add Quorum
    add_quorums(node_config, "node19", quorum_list_file)
    add_quorums(node_config, "node20", quorum_list_file)
    add_quorums(node_config, "node21", quorum_list_file)

    # Setup Quorum
    for did in quorum_did_arr:
        setup_quorum(did, node_config_19["server"], node_config_19["grpcPort"])

    for alias, did in node_config_19["dids"].items():
        if alias.startswith("did_q_"):
            add_peer_details(node_config_19["peerId"], did, 4, node_config_20["server"], node_config_20["grpcPort"])
            add_peer_details(node_config_19["peerId"], did, 4, node_config_21["server"], node_config_21["grpcPort"])

    print("Quorums have been configured")

    # Add Sender and reciever

    create_and_register_did(node_config_19, "did_S1", register_did=False)
    create_and_register_did(node_config_19, "did_R1", register_did=False)

    create_and_register_did(node_config_20, "did_S2", register_did=False)
    create_and_register_did(node_config_20, "did_R2", register_did=False)

    did_S3 = create_and_register_did(node_config_20, "did_S3", register_did=False)
    did_R3 = create_and_register_did(node_config_21, "did_R3", register_did=False)
    add_peer_details(node_config_21["peerId"], did_R3, 4, node_config_20["server"], node_config_20["grpcPort"])
    add_peer_details(node_config_20["peerId"], did_S3, 4, node_config_21["server"], node_config_21["grpcPort"])

    did_S4 = create_and_register_did(node_config_19, "did_S4", register_did=False)
    did_R4 = create_and_register_did(node_config_20, "did_R4", register_did=False)
    add_peer_details(node_config_20["peerId"], did_R4, 4, node_config_19["server"], node_config_19["grpcPort"])
    add_peer_details(node_config_19["peerId"], did_S4, 4, node_config_20["server"], node_config_20["grpcPort"])

def node_config_multiple_same_func(node_config):
    build_dir = get_build_dir()
    quorum_list_file = f"../{build_dir}/quorumlist_multiple_nodes.json"

    node_config_19 = node_config["node19"]
    node_config_20 = node_config["node20"]
    node_config_21 = node_config["node21"]
    node_config_22 = node_config["node22"]

    # Configure Quorum DIDs
    # Serv 1 (Node19): Q1, Q2, Q3
    # Serv 2 (Node20): Q4, Q5, Q6, Q7
    quorum_did_arr = []
    for i in range(1, 8):
        did = ""
        
        if i <= 3:
            did = create_and_register_did(node_config_19, f"did_T2_q_{i}", register_did=False)
            setup_quorum(did, node_config_19["server"], node_config_19["grpcPort"])
            quorum_did_arr.append(did)

            fund_did_with_rbt(node_config_19, did, 20)
        else:
            did = create_and_register_did(node_config_20, f"did_T2_q_{i}", register_did=False)
            setup_quorum(did, node_config_20["server"], node_config_20["grpcPort"])
            quorum_did_arr.append(did)

            fund_did_with_rbt(node_config_20, did, 20)

        # Adding peer details of quorum dids across all three nodes
        if i <= 3:
            add_peer_details(node_config_19["peerId"], did, 4, node_config_20["server"], node_config_20["grpcPort"])
            add_peer_details(node_config_19["peerId"], did, 4, node_config_21["server"], node_config_21["grpcPort"])
            add_peer_details(node_config_19["peerId"], did, 4, node_config_22["server"], node_config_22["grpcPort"])
        else:
            add_peer_details(node_config_20["peerId"], did, 4, node_config_19["server"], node_config_19["grpcPort"])
            add_peer_details(node_config_20["peerId"], did, 4, node_config_21["server"], node_config_21["grpcPort"])
            add_peer_details(node_config_20["peerId"], did, 4, node_config_22["server"], node_config_22["grpcPort"])

    # Create quorumlist config file
    quorum_list = []
    for did in quorum_did_arr:
        quorum_info = {
            "type": 2,
            "address": did
        }
        
        quorum_list.append(quorum_info)

    save_to_config_file(quorum_list_file, quorum_list)
    
    # Add Quorum
    add_quorums(node_config, "node19", quorum_list_file)
    add_quorums(node_config, "node20", quorum_list_file)
    add_quorums(node_config, "node21", quorum_list_file)
    add_quorums(node_config, "node22", quorum_list_file)

    print("Quorums have been configured")

    # Add Sender and reciever

    # Sender and Receiver on a Quorum based server
    create_and_register_did(node_config_19, "did_T2_S1", register_did=False)
    create_and_register_did(node_config_19, "did_T2_R1", register_did=False)

    # Sender and Receiver on a Non-Quorum based server
    create_and_register_did(node_config_21, "did_T2_S2", register_did=False)
    create_and_register_did(node_config_21, "did_T2_R2", register_did=False)

    # Sender on a Quorum and Receiver on a Non-Quorum based server
    did_S3 = create_and_register_did(node_config_20, "did_T2_S3", register_did=False)
    did_R3 = create_and_register_did(node_config_21, "did_T2_R3", register_did=False)
    add_peer_details(node_config_21["peerId"], did_R3, 4, node_config_20["server"], node_config_20["grpcPort"])
    add_peer_details(node_config_20["peerId"], did_S3, 4, node_config_21["server"], node_config_21["grpcPort"])

    # Sender and Receiver on different Non-Quorum based servers
    did_S4 = create_and_register_did(node_config_21, "did_T2_S4", register_did=False)
    did_R4 = create_and_register_did(node_config_22, "did_T2_R4", register_did=False)
    add_peer_details(node_config_22["peerId"], did_R4, 4, node_config_21["server"], node_config_21["grpcPort"])
    add_peer_details(node_config_21["peerId"], did_S4, 4, node_config_22["server"], node_config_22["grpcPort"])

    # Sender and Receiver on different Quorum based servers
    did_S5 = create_and_register_did(node_config_19, "did_T2_S5", register_did=False)
    did_R5 = create_and_register_did(node_config_20, "did_T2_R5", register_did=False)
    add_peer_details(node_config_20["peerId"], did_R5, 4, node_config_19["server"], node_config_19["grpcPort"])
    add_peer_details(node_config_19["peerId"], did_S5, 4, node_config_20["server"], node_config_20["grpcPort"])

def setup():
    print("Setting up test.....")

    node_config = run_nodes()

    # node_config_all_same_func(node_config)
    node_config_multiple_same_func(node_config)

    save_to_config_file(__node_config_path, node_config)

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

    # all_quorums_on_same_node(node_config)
    quorums_on_multiple_nodes(node_config)

    print("\n-------------- Tests Completed -------------------\n")

def all_quorums_on_same_node(config):
    node_19_info, node_20_info, node_21_info = config["node19"], config["node20"], config["node21"]
    
    server_port_19, grpc_port_19 = node_19_info["server"], node_19_info["grpcPort"]
    server_port_20, grpc_port_20 = node_20_info["server"], node_20_info["grpcPort"]
    server_port_21, grpc_port_21 = node_21_info["server"], node_21_info["grpcPort"]
    
    did_S1, did_S2, did_S3, did_S4 = \
          get_did_by_alias(node_19_info, "did_S1"), get_did_by_alias(node_20_info, "did_S2"), \
          get_did_by_alias(node_20_info, "did_S3"), get_did_by_alias(node_19_info, "did_S4")
    
    did_R1, did_R2, did_R3, did_R4 = \
          get_did_by_alias(node_19_info, "did_R1"), get_did_by_alias(node_20_info, "did_R2"), \
          get_did_by_alias(node_21_info, "did_R3"), get_did_by_alias(node_20_info, "did_R4")

    print("------ Test Case (PASS): All Quorums are present on the same node ------\n")
  
    print("\n1.1 Generating 1 whole RBT for S1")
    expect_success(fund_did_with_rbt)(node_19_info, did_S1, 1)
    print("Funded node A with 1 RBT")

    print("\n1.2 Transferring 1 RBT from S1 to R1....")
    expect_success(rbt_transfer)(did_S1, did_R1, 1, server_port_19, grpc_port_19)
    print("Transfer Complete")
    
    print("\n1.3 Transferring 1 RBT from R1 to S1....")
    expect_success(rbt_transfer)(did_R1, did_S1, 1, server_port_19, grpc_port_19)
    print("Transfer Complete")

    print("\n2.1 Generating 1 whole RBT for S2")
    expect_success(fund_did_with_rbt)(node_20_info, did_S2, 1)
    print("Funded node A with 1 RBT")

    print("\n2.2 Transferring 1 RBT from S2 to R2....")
    expect_success(rbt_transfer)(did_S2, did_R2, 1, server_port_20, grpc_port_20)
    print("Transfer Complete")
    
    print("\n2.3 Transferring 1 RBT from R2 to S2....")
    expect_success(rbt_transfer)(did_R2, did_S2, 1, server_port_20, grpc_port_20)
    print("Transfer Complete")

    print("\n3.1 Generating 1 whole RBT for S3")
    expect_success(fund_did_with_rbt)(node_20_info, did_S3, 1)
    print("Funded node A with 1 RBT")

    print("\n3.2 Transferring 1 RBT from S3 to R3....")
    expect_success(rbt_transfer)(did_S3, did_R3, 1, server_port_20, grpc_port_20)
    print("Transfer Complete")
    
    print("\n3.3 Transferring 1 RBT from R3 to S3....")
    expect_success(rbt_transfer)(did_R3, did_S3, 1, server_port_21, grpc_port_21)
    print("Transfer Complete")

    print("\n4.1 Generating 1 whole RBT for S4")
    expect_success(fund_did_with_rbt)(node_19_info, did_S4, 1)
    print("Funded node A with 1 RBT")

    print("\n4.2 Transferring 1 RBT from S4 to R4....")
    expect_success(rbt_transfer)(did_S4, did_R4, 1, server_port_19, grpc_port_19)
    print("Transfer Complete")
    
    print("\n4.3 Transferring 1 RBT from R2 to S2....")
    expect_success(rbt_transfer)(did_R4, did_S4, 1, server_port_20, grpc_port_20)
    print("Transfer Complete")

    print("------ Test Case (PASS): All Quorums are present on the same node completed ------\n")

def quorums_on_multiple_nodes(config):
    import time
    node_19_info, node_20_info, node_21_info, node_22_info = config["node19"], config["node20"], config["node21"], config["node22"]
    
    server_port_19, grpc_port_19 = node_19_info["server"], node_19_info["grpcPort"]
    server_port_20, grpc_port_20 = node_20_info["server"], node_20_info["grpcPort"]
    server_port_21, grpc_port_21 = node_21_info["server"], node_21_info["grpcPort"]
    server_port_22, grpc_port_22 = node_22_info["server"], node_22_info["grpcPort"]
    
    did_S1, did_S2, did_S3, did_S4, did_S5 = \
          get_did_by_alias(node_19_info, "did_T2_S1"), get_did_by_alias(node_21_info, "did_T2_S2"), \
          get_did_by_alias(node_20_info, "did_T2_S3"), get_did_by_alias(node_21_info, "did_T2_S4"), \
          get_did_by_alias(node_19_info, "did_T2_S5")
    
    did_R1, did_R2, did_R3, did_R4, did_R5 = \
          get_did_by_alias(node_19_info, "did_T2_R1"), get_did_by_alias(node_21_info, "did_T2_R2"), \
          get_did_by_alias(node_21_info, "did_T2_R3"), get_did_by_alias(node_22_info, "did_T2_R4"), \
          get_did_by_alias(node_20_info, "did_T2_R5")

    print("------ Test Case (PASS): Quorums are present on the multiple nodes ------\n")
  
    print("\n1.1 Generating 1 whole RBT for S1")
    expect_success(fund_did_with_rbt)(node_19_info, did_S1, 1)
    print("Funded node A with 1 RBT")

    print("\n1.2 Transferring 1 RBT from S1 to R1....")
    expect_success(rbt_transfer)(did_S1, did_R1, 1, server_port_19, grpc_port_19)
    print("Transfer Complete")
    
    # time.sleep(12)
     
    print("\n1.3 Transferring 1 RBT from R1 to S1....")
    expect_success(rbt_transfer)(did_R1, did_S1, 1, server_port_19, grpc_port_19)
    print("Transfer Complete")

    print("\n2.1 Generating 1 whole RBT for S2")
    expect_success(fund_did_with_rbt)(node_21_info, did_S2, 1)
    print("Funded node A with 1 RBT")

    print("\n2.2 Transferring 1 RBT from S2 to R2....")
    expect_success(rbt_transfer)(did_S2, did_R2, 1, server_port_21, grpc_port_21)
    print("Transfer Complete")
    # time.sleep(12)
    print("\n2.3 Transferring 1 RBT from R2 to S2....")
    expect_success(rbt_transfer)(did_R2, did_S2, 1, server_port_21, grpc_port_21)
    print("Transfer Complete")

    print("\n3.1 Generating 1 whole RBT for S3")
    expect_success(fund_did_with_rbt)(node_20_info, did_S3, 1)
    print("Funded node A with 1 RBT")

    print("\n3.2 Transferring 1 RBT from S3 to R3....")
    expect_success(rbt_transfer)(did_S3, did_R3, 1, server_port_20, grpc_port_20)
    print("Transfer Complete")
    # time.sleep(12)
    print("\n3.3 Transferring 1 RBT from R3 to S3....")
    expect_success(rbt_transfer)(did_R3, did_S3, 1, server_port_21, grpc_port_21)
    print("Transfer Complete")

    print("\n4.1 Generating 1 whole RBT for S4")
    expect_success(fund_did_with_rbt)(node_21_info, did_S4, 1)
    print("Funded node A with 1 RBT")

    print("\n4.2 Transferring 1 RBT from S4 to R4....")
    expect_success(rbt_transfer)(did_S4, did_R4, 1, server_port_21, grpc_port_21)
    print("Transfer Complete")
    # time.sleep(12)
    print("\n4.3 Transferring 1 RBT from R4 to S4....")
    expect_success(rbt_transfer)(did_R4, did_S4, 1, server_port_22, grpc_port_22)
    print("Transfer Complete")

    print("\n5.1 Generating 1 whole RBT for S5")
    expect_success(fund_did_with_rbt)(node_19_info, did_S5, 1)
    print("Funded node A with 1 RBT")

    print("\n5.2 Transferring 1 RBT from S5 to R5....")
    expect_success(rbt_transfer)(did_S5, did_R5, 1, server_port_19, grpc_port_19)
    print("Transfer Complete")
    # time.sleep(12)
    print("\n5.3 Transferring 1 RBT from R5 to S5....")
    expect_success(rbt_transfer)(did_R5, did_S5, 1, server_port_20, grpc_port_20)
    print("Transfer Complete")

    print("------ Test Case (PASS): Quorums are present on the multiple nodes completed ------\n")


