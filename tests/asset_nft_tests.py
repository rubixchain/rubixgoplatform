import os
import time
from node.actions import fund_did_with_rbt, setup_rubix_nodes, \
    create_and_register_did, add_quorums, check_if_node_is_running, \
    register_quorums_from_config, generate_nft, deploy_nft, self_execute_nft, transfer_nft, get_nft_chain, \
    add_peer_details
from node.utils import get_did_by_alias, update_sample_artifact_file
from config.utils import save_to_config_file, load_from_config_file
from helper.utils import expect_success
from node.quorum import run_quorum_nodes
from node.commands import run_command
from prerequisite import get_os_info

__quorum_list_file_name = "quorumlist_nft.json"
__quorum_config_file_name = "quorum_config_nft.json"

def boot_quorums():
    run_quorum_nodes(
        False, 
        skip_adding_quorums=False,
        node_registry_key="quorum_nft", 
        quorum_list_file_name=__quorum_list_file_name, 
        quorum_config_path=__quorum_config_file_name,
        only_bip39=True,
        concurrent=True
    )

def setup(is_mac_os=False):
    nft_node_config = setup_rubix_nodes("nodes_nft", concurrent=True)

    for node, config in nft_node_config.items():
        if not check_if_node_is_running(int(node.lstrip("node"))):
            raise Exception(f"{node} is NOT running. Exiting...")
        print(f"{node} is running.")

    if is_mac_os:
        quorum_config = load_from_config_file(__quorum_config_file_name)
        for node, nq_config in nft_node_config.items():
            for _, node_config in quorum_config.items():
                add_peer_details(
                    node_config["peerId"],
                    node_config["dids"]["did_quorum"]["did"],
                    4,
                    nq_config["server"],
                    nq_config["grpcPort"]
                )

    create_and_register_did(nft_node_config["node31"], "didA", register_did=True)
    create_and_register_did(nft_node_config["node31"], "didA2", register_did=True)

    save_to_config_file("nft_nodes_config.json", nft_node_config)

    print("Adding quorum for node31")
    add_quorums(nft_node_config, "node31", quorumlist=__quorum_list_file_name)

    
def tests():
    # Re-register Quorum DIDs so that all the nodes have info about peerID of Quorum DIDs
    register_quorums_from_config(__quorum_config_file_name)

    current_working_dir = os.getcwd()
    valid_artifact_path = os.path.abspath(os.path.join(current_working_dir, "fixtures", "valid_nft", "artifact.csv"))
    valid_metadata_path = os.path.abspath(os.path.join(current_working_dir, "fixtures", "valid_nft", "metadata"))
 
    update_sample_artifact_file(valid_artifact_path)

    sc_node_config = load_from_config_file("nft_nodes_config.json")

    didA = get_did_by_alias(sc_node_config["node31"], "didA")
    didA2 = get_did_by_alias(sc_node_config["node31"], "didA2")

    didA_port = sc_node_config["node31"]["server"]

    print("\n1. Generating 1 whole RBT for A1")
    expect_success(fund_did_with_rbt)(sc_node_config["node31"], didA, 1)
    print("Funded did A with 1 RBT")
    
    print("\n1. Generate an NFT")
    nft_id = generate_nft(didA, valid_artifact_path, valid_metadata_path, didA_port)
    print("Generated NFT: ", nft_id)

    print("\n2. Deploy an NFT")
    expect_success(deploy_nft)(nft_id, didA, 0.001, didA_port)

    print("\n3. Execute an NFT on the same node")
    expect_success(self_execute_nft)(nft_id, didA, 0.001, didA_port)

    print("\n4. Asserting if the length of NFT tokenchain is 2")
    nft_token_chain = get_nft_chain(nft_id, didA_port)
    assert len(nft_token_chain) == 2, "expected length of NFT token chain to be 2"

    print("\n5. Transfer ownership of NFT")
    expect_success(transfer_nft)(nft_id, didA, didA2, 0.1, didA_port)

    print("\n6. Asserting if the length of NFT tokenchain is 3")
    nft_token_chain = get_nft_chain(nft_id, didA_port)
    assert len(nft_token_chain) == 3, "expected length of NFT token chain to be 3"

    print(f"\n7. Assetint if {didA2} is the new owner of the NFT")
    nft_token_chain = get_nft_chain(nft_id, didA_port, latest_block=True)
    latest_nft_block = nft_token_chain[0]
    current_owner = latest_nft_block["NFTOwner"]
    assert current_owner == didA2, f"expected current owner of NFT to be {didA2}, found {current_owner}"

if __name__=='__main__':
    boot_quorums()

    _, os_name = get_os_info()

    if os_name == 'mac':
        setup(is_mac_os=True)
    else:
        setup(is_mac_os=False)

    tests()