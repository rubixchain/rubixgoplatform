from node.actions import rbt_transfer, fund_did_with_rbt
from helper.utils import expect_failure, expect_success
from .util import get_non_quorum_node_configs

def max_decimal_place_transfer(config):
    node_A_info, node_B_info = config["nodeNq14"], config["nodeNq15"]

    print("------ Test Case (FAIL) : Transferring 0.00000009 RBT from B which is more than allowed decimal places ------")

    print("\nTransferring 0.00000009 RBT from B to A....")
    expect_failure(rbt_transfer)(node_B_info, node_A_info, 0.00000009)

    print("\n------ Test Case (FAIL) : Transferring 0.00000009 RBT from B which is more than allowed decimal places completed ------\n")

def insufficient_balance_transfer(config):
    node_A_info, node_B_info = config["nodeNq14"], config["nodeNq15"]

    print("\n------ Test Case (FAIL) : Transferring 100 RBT from A which has zero balance ------")

    print("\nTransferring 100 RBT from A to B....")
    expect_failure(rbt_transfer)(node_A_info, node_B_info, 100)

    print("\n------ Test Case (FAIL) : Transferring 100 RBT from A which has zero balance completed ------\n")


    print("\n------ Test Case (FAIL) : Transferring 100 RBT from B which has insufficient balance ------")

    print("\nTransferring 100 RBT from B to A....")
    expect_failure(rbt_transfer)(node_B_info, node_A_info, 100)

    print("\n------ Test Case (FAIL) : Transferring 100 RBT from B which has insufficient balance completed ------\n")

def shuttle_transfer(config):
    node_A_info, node_B_info = config["nodeNq14"], config["nodeNq15"]

    print("------ Test Case (PASS): Shuttle transfer started ------\n")

    print("\n1. Generating 2 whole RBT for A")
    expect_success(fund_did_with_rbt)(node_A_info, 2)
    print("Funded node A with 2 RBT")

    print("\n2. Transferring 0.5 RBT from A to B....")
    expect_success(rbt_transfer)(node_A_info, node_B_info, 0.5)
    print("Transferred 0.5 RBT from A to B")

    print("\n3. Transferring 1.499 RBT from A to B....")
    expect_success(rbt_transfer)(node_A_info, node_B_info, 1.499)
    print("Transferred 1.499 RBT from A to B")

    print("\n4. Transferring 0.25 RBT from B to A....")
    expect_success(rbt_transfer)(node_B_info, node_A_info, 0.25)
    print("Transferred 0.25 RBT from B to A")

    print("\n5. Transferring 0.25 RBT from B to A....")
    expect_success(rbt_transfer)(node_B_info, node_A_info, 0.25)
    print("Transferred 0.25 RBT from B to A")

    print("\n6. Transferring 0.25 RBT from B to A....")
    expect_success(rbt_transfer)(node_B_info, node_A_info, 0.25)
    print("Transferred 0.25 RBT from B to A")

    print("\n7. Transferring 0.25 RBT from B to A....")
    expect_success(rbt_transfer)(node_B_info, node_A_info, 0.25)
    print("Transferred 0.25 RBT from B to A")

    print("\n8. Transferring 1 RBT from A to B....")
    expect_success(rbt_transfer)(node_A_info, node_B_info, 1)
    print("Transferred 1 RBT from A to B")    

    print("\n9. Generating 2 whole RBT for A")
    expect_success(fund_did_with_rbt)(node_A_info, 2)
    print("Funded node A with 2 RBT")
    
    print("\n10. Transferring 2 RBT from A to B....")
    expect_success(rbt_transfer)(node_A_info, node_B_info, 2)
    print("Transferred 2 RBT from A to B")    

    print("\n11. Transferring 0.001 RBT from A to B....")
    expect_success(rbt_transfer)(node_A_info, node_B_info, 0.001)
    print("Transferred 0.001 RBT from A to B")

    print("\n------ Test Case (PASS): Shuttle transfer completed ------\n")
