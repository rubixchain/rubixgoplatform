import os
import time
from node.actions import generate_smart_contract, rbt_transfer, fund_did_with_rbt, setup_rubix_nodes, \
    create_and_register_did, add_quorums, add_peer_details, check_if_node_is_running, mint_ft, \
    transfer_ft, register_quorums_from_config, deploy_smart_contract, execute_smart_contract, \
    get_smart_contract_chain
from node.utils import get_did_by_alias, update_sample_rust_file
from config.utils import save_to_config_file, load_from_config_file
from helper.utils import expect_failure, expect_success
from node.quorum import get_quorum_config, run_quorum_nodes
from constants import IPFS_KUBO_VERSION
from prerequisite import get_testnet_ipfs_swarm_key, get_os_info, download_ipfs_binary, copy_fixtures_to_build_dir
from node.commands import run_command


__quorum_list_file_name = "quorumlist_sc.json"
__quorum_config_file_name = "quorum_config_sc.json"

def load_prerequisite():
    os_name, build_folder = get_os_info()
    if os_name is None:
        exit(1)

    os.chdir("../")
    print(f"Building Rubix binary for {os_name}\n")
    build_command = ""
    if os_name == "Linux":
        build_command = "make compile-linux"
    elif os_name == "Windows":
        build_command = "make compile-windows"
    elif os_name == "Darwin":
        build_command = "make compile-mac"
    
    output, code = run_command(build_command)
    if code != 0:
        print("build failed with error:", output)
        exit(1)
    else:
        print("\nBuild successful\n")

    get_testnet_ipfs_swarm_key(build_folder)
    download_ipfs_binary(os_name, IPFS_KUBO_VERSION, build_folder)
    copy_fixtures_to_build_dir(build_folder)
   
    os.chdir("./tests") 


def boot_quorums():
    run_quorum_nodes(
        False, 
        skip_adding_quorums=False,
        node_registry_key="quorum_smart_contract", 
        quorum_list_file_name=__quorum_list_file_name, 
        quorum_config_path=__quorum_config_file_name,
        only_bip39=True,
        concurrent=True
    )

def setup():
    smart_contract_node_config = setup_rubix_nodes("nodes_smart_contract", concurrent=True)

    for node, config in smart_contract_node_config.items():
        if not check_if_node_is_running(int(node.lstrip("node"))):
            raise Exception(f"{node} is NOT running. Exiting...")
        print(f"{node} is running.")

    create_and_register_did(smart_contract_node_config["node22"], "didA", register_did=True)

    save_to_config_file("smart_contract_nodes_config.json", smart_contract_node_config)

    print("Adding quorum for node22")
    add_quorums(smart_contract_node_config, "node22", quorumlist=__quorum_list_file_name)

    # register all the quorum dids
    
def tests():
    # Re-register Quorum DIDs so that all the nodes have info about peerID of Quorum DIDs
    register_quorums_from_config(__quorum_config_file_name)

    current_working_dir = os.getcwd()
    valid_wasm_path = os.path.abspath(os.path.join(current_working_dir, "fixtures", "valid_sc", "contract.wasm"))
    valid_code_path = os.path.abspath(os.path.join(current_working_dir, "fixtures", "valid_sc", "lib.rs"))

    update_sample_rust_file(valid_code_path)


    sc_node_config = load_from_config_file("smart_contract_nodes_config.json")

    didA = get_did_by_alias(sc_node_config["node22"], "didA")

    didA_port = sc_node_config["node22"]["server"]

    print("\n1. Generating 1 whole RBT for A1")
    expect_success(fund_did_with_rbt)(sc_node_config["node22"], didA, 1)
    print("Funded node A with 1 RBT")
    
    print("\n1. Generate a Smart Contract")
    sc_hash = generate_smart_contract(didA, valid_code_path, valid_wasm_path, didA_port)
    print("Generated smart contract: ", sc_hash)

    print("\n2. Deploy a smart Contract")
    expect_success(deploy_smart_contract)(sc_hash, didA, 0.001, didA_port)

    print("\n3. Execute a smart Contract on the same node")
    expect_success(execute_smart_contract)(sc_hash, didA, didA_port, sctData="stats")
    
    print("\n4. Asserting if the length of Smart Contract is 2")
    sct_token_chain = get_smart_contract_chain(sc_hash, didA_port)
    assert len(sct_token_chain) == 2, "expected length of smart contract token chain to be 2"

if __name__=='__main__':
    # load_prerequisite()

    boot_quorums()

    setup()

    tests()