import subprocess
import os
import re
import platform
import time
import requests
from .utils import get_base_ports, extract_cid_v0_from_msg

def is_windows_os():
    os_name = platform.system()
    return os_name == "Windows"

def get_build_dir():
    os_name = platform.system()
    build_folder = ""
    if os_name == "Linux":
        build_folder = "linux"
    elif os_name == "Windows":
        build_folder = "windows"
    elif os_name == "Darwin":
        build_folder = "mac"

    return build_folder

def run_command(cmd_string, is_output_from_stderr=False):
    assert isinstance(cmd_string, str), "command must be of string type"
    cmd_result = subprocess.run(cmd_string, stdout=subprocess.PIPE, stderr=subprocess.PIPE, shell=True)
    code = cmd_result.returncode
    
    if int(code) != 0:
        err_output = cmd_result.stderr.decode('utf-8')[:-1]
        print(err_output)
        return err_output, int(code)

    output = ""
    if not is_output_from_stderr:
        output = cmd_result.stdout.decode('utf-8')[:-1]
        print(output)
        if output.find('[ERROR]') > 0 or output.find('parse error') > 0:
            return output, 1
        else:
            return output, code
    else:
        output = cmd_result.stderr.decode('utf-8')[:-1]
        if output.find('[ERROR]') > 0 or output.find('parse error') > 0:
            print(output)
            return output, 1
        else:
            return output, code

def cmd_run_rubix_servers(node_name, server_port_idx, concurrent=False):
    os.chdir("../" + get_build_dir())
    
    base_node_server, base_grpc_port = get_base_ports()
    grpc_port = base_grpc_port + server_port_idx
    node_server = base_node_server + server_port_idx

    cmd_string = ""
    if is_windows_os():
        cmd_string = f"powershell -Command  Start-Process -FilePath '.\\rubixgoplatform.exe' -ArgumentList 'run -p {node_name} -n {server_port_idx} -s -testNet -grpcPort {grpc_port}'"
    else:
        cmd_string = f"tmux new -s {node_name} -d ./rubixgoplatform run -p {node_name} -n {server_port_idx} -s -testNet -grpcPort {grpc_port}"
    
    _, code = run_command(cmd_string)
    if code != 0:
        raise Exception("Error occurred while run the command: " + cmd_string)
    

    if not concurrent:
        print("Waiting for 80 seconds before checking if its running....")
        time.sleep(80)
        try:
            check_if_node_is_running(server_port_idx)
        except Exception as e:
            raise e
        
        os.chdir("../tests")
    
    return node_server, grpc_port

def check_if_node_is_running(server_idx):
    base_server, _ = get_base_ports()
    port = base_server + int(server_idx)
    print(f"Check if server with ENS web server port {port} is running...")
    url = f"http://localhost:{port}/api/getalldid"
    try:
        print(f"Sending GET request to URL: {url}")
        response = requests.get(url)
        if response.status_code == 200:
            print(f"Server with port {port} is running successfully")
            return True
        else:
            raise Exception(f"Failed with Status Code: {response.status_code} |  Server with port {port} is NOT running successfully")
    except:
        raise Exception(f"ConnectionError | Server with port {port} is NOT running successfully")

def cmd_create_did(server_port, grpc_port, did_type = 4, priv_pwd = "mypassword", quorum_pwd = "mypassword"):
    os.chdir("../" + get_build_dir())
    did_type = 4 # TODO: temporaray change until -didType flag is removed
    cmd_string = f"./rubixgoplatform createdid -port {server_port} -grpcPort {grpc_port} -didType {did_type} -privPWD {priv_pwd} -quorumPWD {quorum_pwd}"
    if is_windows_os():
        cmd_string = f".\\rubixgoplatform createdid -port {server_port} -grpcPort {grpc_port} -didType {did_type} -privPWD {priv_pwd} -quorumPWD {quorum_pwd}"
    output, code = run_command(cmd_string, True)
    print(output)
    
    if code != 0:
        raise Exception("Error occurred while run the command: " + cmd_string)
    
    did_id = ""
    if "successfully" in output:
        pattern = r'bafybmi\w+'
        matches = re.findall(pattern, output)
        if matches:
            did_id = matches[0]
        else:
            raise Exception("unable to extract DID ID")

    os.chdir("../tests")
    return did_id

def cmd_register_did(did_id, server_port, grpc_port, priv_pwd = "mypassword"):
    os.chdir("../" + get_build_dir())
    cmd_string = f"./rubixgoplatform registerdid -did {did_id} -port {server_port} -grpcPort {grpc_port} -privPWD {priv_pwd}"
    if is_windows_os():
        cmd_string = f".\\rubixgoplatform registerdid -did {did_id} -port {server_port} -grpcPort {grpc_port} -privPWD {priv_pwd}"
    output, code = run_command(cmd_string, True)
    print(output)

    if code != 0:
        raise Exception("Error occurred while run the command: " + cmd_string)

    os.chdir("../tests")
    return output

def cmd_add_peer_details(peer_id, did_id, did_type, server_port, grpc_port):
    os.chdir("../" + get_build_dir())
    cmd_string = f"./rubixgoplatform addpeerdetails -peerID {peer_id} -did {did_id} -didType {did_type} -port {server_port} -grpcPort {grpc_port}"
    if is_windows_os():
        cmd_string = f".\\rubixgoplatform addpeerdetails -peerID {peer_id} -did {did_id} -didType {did_type} -port {server_port} -grpcPort {grpc_port}"
    output, code = run_command(cmd_string, True)
    print(output)

    if code != 0:
        raise Exception("Error occurred while run the command: " + cmd_string)

    os.chdir("../tests")
    return output

def cmd_generate_rbt(did_id, numTokens, server_port, grpc_port, start_index=0, priv_pwd = "mypassword"):
    os.chdir("../" + get_build_dir())
    cmd_string = f"./rubixgoplatform generatetestrbt -did {did_id} -numTokens {numTokens} -port {server_port} -startIndex {start_index} -grpcPort {grpc_port} -privPWD {priv_pwd}"
    if is_windows_os():
        cmd_string = f".\\rubixgoplatform generatetestrbt -did {did_id} -numTokens {numTokens} -port {server_port} -startIndex {start_index} -grpcPort {grpc_port} -privPWD {priv_pwd}"
    output, code = run_command(cmd_string, True)
    
    if code != 0:
        raise Exception("Error occurred while run the command: " + cmd_string)

    os.chdir("../tests")
    return output

def cmd_add_quorum_dids(server_port, grpc_port, quorumlist = "quorumlist.json"):
    os.chdir("../" + get_build_dir())
    cmd_string = f"./rubixgoplatform addquorum -port {server_port} -grpcPort {grpc_port} -quorumList {quorumlist}"
    if is_windows_os():
        cmd_string = f".\\rubixgoplatform addquorum -port {server_port} -grpcPort {grpc_port} -quorumList {quorumlist}"
    output, code = run_command(cmd_string, True)
    print(output)
    if code != 0:
        raise Exception("Error occurred while run the command: " + cmd_string)

    os.chdir("../tests")
    return output

def cmd_shutdown_node(server_port, grpc_port):
    os.chdir("../" + get_build_dir())
    cmd_string = f"./rubixgoplatform shutdown -port {server_port} -grpcPort {grpc_port}"
    if is_windows_os():
        cmd_string = f".\\rubixgoplatform shutdown -port {server_port} -grpcPort {grpc_port}"
    output, _ = run_command(cmd_string, True)
    print(output)

    os.chdir("../tests")
    return output

def cmd_setup_quorum_dids(did, server_port, grpc_port, priv_pwd, quorum_pwd):
    os.chdir("../" + get_build_dir())
    cmd_string = f"./rubixgoplatform setupquorum -did {did} -port {server_port} -grpcPort {grpc_port} -privPWD {priv_pwd} -quorumPWD {quorum_pwd}"
    if is_windows_os():
        cmd_string = f".\\rubixgoplatform setupquorum -did {did} -port {server_port} -grpcPort {grpc_port} -privPWD {priv_pwd} -quorumPWD {quorum_pwd}"
    output, code = run_command(cmd_string, True)
    print(output)
    if code != 0:
        raise Exception("Error occurred while run the command: " + cmd_string)

    os.chdir("../tests")
    return output

def cmd_get_peer_id(server_port, grpc_port):
    os.chdir("../" + get_build_dir())
    cmd_string = f"./rubixgoplatform get-peer-id -port {server_port} -grpcPort {grpc_port}"
    if is_windows_os():
        cmd_string = f".\\rubixgoplatform get-peer-id -port {server_port} -grpcPort {grpc_port}"
    output, code = run_command(cmd_string)

    if code != 0:
        raise Exception("Error occurred while run the command: " + cmd_string)
    os.chdir("../tests")
    return output

def check_account_info(did, server_port, grpc_port, priv_pwd = "mypassword"):
    os.chdir("../" + get_build_dir())
    cmd_string = f"./rubixgoplatform getaccountinfo -did {did} -port {server_port} -grpcPort {grpc_port} -privPWD {priv_pwd}"
    if is_windows_os():
        cmd_string = f".\\rubixgoplatform getaccountinfo -did {did} -port {server_port} -grpcPort {grpc_port} -privPWD {priv_pwd}"
    output, code = run_command(cmd_string)

    if code != 0:
        raise Exception("Error occurred while run the command: " + cmd_string)
    os.chdir("../tests")
    return output

# Note: address != did, address = peerId.didId 
def cmd_rbt_transfer(sender_address, receiver_address, rbt_amount, server_port, grpc_port, priv_pwd = "mypassword"):
    os.chdir("../" + get_build_dir())
    cmd_string = f"./rubixgoplatform transferrbt -senderAddr {sender_address} -receiverAddr {receiver_address} -rbtAmount {rbt_amount} -port {server_port} -grpcPort {grpc_port} -privPWD {priv_pwd}"
    if is_windows_os():
        cmd_string = f".\\rubixgoplatform transferrbt -senderAddr {sender_address} -receiverAddr {receiver_address} -rbtAmount {rbt_amount} -port {server_port} -grpcPort {grpc_port} -privPWD {priv_pwd}"
    output, code = run_command(cmd_string, True)
    print(output)
    if code != 0:
        raise Exception("Error occurred while run the command: " + cmd_string)

    os.chdir("../tests")
    return output

def cmd_mint_ft(did: str, ftCount: int, ftName: str, rbtAmount: int, server_port,   priv_pwd="mypassword"):
    os.chdir("../" + get_build_dir())
    cmd_string = f"./rubixgoplatform create-ft -did {did} -ftCount {ftCount} -ftName {ftName} -rbtAmount {rbtAmount} -port {server_port} -privPWD {priv_pwd}"
    if is_windows_os():
        cmd_string = f".\\rubixgoplatform create-ft -did {did} -ftCount {ftCount} -ftName {ftName} -rbtAmount {rbtAmount} -port {server_port} -privPWD {priv_pwd}"
    output, code = run_command(cmd_string, True)
    print(output)
    if code != 0:
        raise Exception("Error occurred while run the command: " + cmd_string)

    os.chdir("../tests")
    print(f"DID {did} is minted with {ftCount} FT")

def cmd_transfer_ft(senderDid: str, receiverDid: str, creatorDid: str, ftName: str, ftCount: int, server_port, priv_pwd="mypassword"):
    os.chdir("../" + get_build_dir())
    cmd_string = f"./rubixgoplatform transfer-ft -senderAddr {senderDid} -receiverAddr {receiverDid} -creatorDID {creatorDid} -ftName {ftName} -ftCount {ftCount} -port {server_port} -privPWD {priv_pwd}"
    if is_windows_os():
        cmd_string = f".\\rubixgoplatform transfer-ft -senderAddr {senderDid} -receiverAddr {receiverDid} -creatorDID {creatorDid} -ftName {ftName} -ftCount {ftCount} -port {server_port} -privPWD {priv_pwd}"
    output, code = run_command(cmd_string, True)
    print(output)
    if code != 0:
        raise Exception("Error occurred while run the command: " + cmd_string)

    os.chdir("../tests")
    return output

# NFT

def cmd_create_nft(artifact_path: str, metadata_path: str, did: str, server_port):
    os.chdir("../" + get_build_dir())
    cmd_string = f"./rubixgoplatform create-nft -artifact {artifact_path} -metadata {metadata_path} -did {did} -port {server_port}"
    if is_windows_os():
        cmd_string = f".\\rubixgoplatform create-nft -artifact {artifact_path} -metadata {metadata_path} -did {did} -port {server_port}"
    output, code = run_command(cmd_string, True)
    print(output)
    if code != 0:
        raise Exception("Error occurred while run the command: " + cmd_string)

    os.chdir("../tests")

def cmd_deploy_nft(deployer_addr, nft_id, nft_value, server_port):
    os.chdir("../" + get_build_dir())
    cmd_string = f"./rubixgoplatform deploy-nft -deployerAddr {deployer_addr} -nft {nft_id} -nftValue {nft_value} -port {server_port}"
    if is_windows_os():
        cmd_string = f".\\rubixgoplatform deploy-nft -deployerAddr {deployer_addr} -nft {nft_id} -nftValue {nft_value} -port {server_port}"
    print(cmd_string)
    output, code = run_command(cmd_string, True)
    print(output)
    if code != 0:
        raise Exception("Error occurred while run the command: " + cmd_string)

    os.chdir("../tests")

def cmd_self_execute_nft(executor_did, nft_id, nft_value, server_port):
    os.chdir("../" + get_build_dir())
    cmd_string = f"./rubixgoplatform execute-nft -executorAddr {executor_did} -nft {nft_id} -nftValue {nft_value} -port {server_port}"
    if is_windows_os():
        cmd_string = f".\\rubixgoplatform execute-nft -executorAddr {executor_did} -nft {nft_id} -nftValue {nft_value} -port {server_port}"
    output, code = run_command(cmd_string, True)
    print(output)
    if code != 0:
        raise Exception("Error occurred while run the command: " + cmd_string)

    os.chdir("../tests")

def cmd_transfer_nft(executor_did, receiver_did, nft_id, nft_value, server_port):
    os.chdir("../" + get_build_dir())
    cmd_string = f"./rubixgoplatform execute-nft -executorAddr {executor_did} -receiverAddr {receiver_did} -nft {nft_id} -nftValue {nft_value} -port {server_port}"
    if is_windows_os():
        cmd_string = f".\\rubixgoplatform execute-nft -executorAddr {executor_did} -receiverAddr {receiver_did} -nft {nft_id} -nftValue {nft_value} -port {server_port}"
    output, code = run_command(cmd_string, True)
    print(output)
    if code != 0:
        raise Exception("Error occurred while run the command: " + cmd_string)

    os.chdir("../tests")

def cmd_subscribe_nft(nft_id, server_port):
    os.chdir("../" + get_build_dir())
    cmd_string = f"./rubixgoplatform subscribe-nft -sct {nft_id} -port {server_port}"
    if is_windows_os():
        cmd_string = f".\\rubixgoplatform subscribe-nft -sct {nft_id} -port {server_port}"
    output, code = run_command(cmd_string, True)
    print(output)

    if code != 0:
        raise Exception("Error occurred while run the command: " + cmd_string)
    
    os.chdir("../tests")




# Smart contract
def cmd_generate_smart_contract(did: str, raw_code_path: str, bin_code_path: str, server_port, priv_pwd="mypassword"):
    os.chdir("../" + get_build_dir())
    cmd_string = f"./rubixgoplatform generatesct -did {did} -binCode {bin_code_path} -rawCode {raw_code_path} -port {server_port}"
    if is_windows_os():
        cmd_string = f".\\rubixgoplatform generatesct -did {did} -binCode {bin_code_path} -rawCode {raw_code_path} -port {server_port}"
    output, code = run_command(cmd_string, True)
    if code != 0:
        raise Exception("Error occurred while run the command: " + cmd_string)
    
    # Extract out the CID
    cid = extract_cid_v0_from_msg(output)

    os.chdir("../tests")
    return cid

def cmd_deploy_smart_contract(smart_contract_id: str, did: str, rbtAmount: float, server_port, priv_pwd="mypassword"):
    os.chdir("../" + get_build_dir())
    cmd_string = f"./rubixgoplatform deploysmartcontract -sct {smart_contract_id} -deployerAddr {did} -rbtAmount {rbtAmount} -port {server_port}"
    if is_windows_os():
        cmd_string = f".\\rubixgoplatform deploysmartcontract -sct {smart_contract_id} -deployerAddr {did} -rbtAmount {rbtAmount} -port {server_port}"
    output, code = run_command(cmd_string, True)
    print(output)
    if code != 0:
        raise Exception("Error occurred while run the command: " + cmd_string)
    

    os.chdir("../tests")

def cmd_execute_smart_contract(smart_contract_id: str, did: str, server_port, priv_pwd="mypassword", sctData="test"):
    os.chdir("../" + get_build_dir())
    cmd_string = f"./rubixgoplatform executesmartcontract -sct {smart_contract_id} -executorAddr {did} -sctData {sctData} -port {server_port}"
    if is_windows_os():
        cmd_string = f".\\rubixgoplatform executesmartcontract -sct {smart_contract_id} -executorAddr {did} -sctData {sctData} -port {server_port}"
    output, code = run_command(cmd_string, True)
    print(output)
    if code != 0:
        raise Exception("Error occurred while run the command: " + cmd_string)
    

    os.chdir("../tests")

def cmd_subscribe_smart_contract(smart_contract_id: str, server_port):
    os.chdir("../" + get_build_dir())
    cmd_string = f"./rubixgoplatform subscribesct -sct {smart_contract_id} -port {server_port}"
    if is_windows_os():
        cmd_string = f".\\rubixgoplatform subscribesct -sct {smart_contract_id} -port {server_port}"
    output, code = run_command(cmd_string, True)
    print(output)

    if code != 0:
        raise Exception("Error occurred while run the command: " + cmd_string)
    
    os.chdir("../tests")


