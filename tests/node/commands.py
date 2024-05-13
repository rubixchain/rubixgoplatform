import subprocess
import os
import re
import platform
import time
import requests

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

def cmd_run_rubix_servers(node_name, server_port_idx, grpc_port):
    os.chdir("../" + get_build_dir())
    
    cmd_string = ""
    if is_windows_os():
        cmd_string = f"powershell -Command  Start-Process -FilePath '.\\rubixgoplatform.exe' -ArgumentList 'run -p {node_name} -n {server_port_idx} -s -testNet -grpcPort {grpc_port}' -WindowStyle Hidden"
    else:
        cmd_string = f"tmux new -s {node_name} -d ./rubixgoplatform run -p {node_name} -n {server_port_idx} -s -testNet -grpcPort {grpc_port}"
    
    _, code = run_command(cmd_string)
    if code != 0:
        raise Exception("Error occurred while run the command: " + cmd_string)
    
    print("Waiting for 60 seconds before checking if its running....")
    time.sleep(60)
    try:
        check_if_all_nodes_are_running(server_port_idx)
    except Exception as e:
        raise e
    os.chdir("../tests")

def check_if_all_nodes_are_running(server_idx):
    
    base_server = 20000
    port = base_server + int(server_idx)
    print(f"Check if server with ENS web server port {port} is running...")
    url = f"http://localhost:{port}/api/getalldid"
    try:
        print(f"Sending GET request to URL: {url}")
        response = requests.get(url)
        if response.status_code == 200:
            print(f"Server with port {port} is running successfully")
        else:
            raise Exception(f"Failed with Status Code: {response.status_code} |  Server with port {port} is NOT running successfully")
    except:
        raise Exception(f"ConnectionError | Server with port {port} is NOT running successfully")

def cmd_create_did(server_port, grpc_port):
    os.chdir("../" + get_build_dir())

    cmd_string = f"./rubixgoplatform createdid -port {server_port} -grpcPort {grpc_port}"
    if is_windows_os():
        cmd_string = f".\\rubixgoplatform createdid -port {server_port} -grpcPort {grpc_port}"
    output, code = run_command(cmd_string, True)

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

def cmd_register_did(did_id, server_port, grpc_port):
    os.chdir("../" + get_build_dir())
    cmd_string = f"./rubixgoplatform registerdid -did {did_id} -port {server_port} -grpcPort {grpc_port}"
    if is_windows_os():
        cmd_string = f".\\rubixgoplatform registerdid -did {did_id} -port {server_port} -grpcPort {grpc_port}"
    output, code = run_command(cmd_string)
    print(output)

    if code != 0:
        raise Exception("Error occurred while run the command: " + cmd_string)

    os.chdir("../tests")
    return output

def cmd_generate_rbt(did_id, numTokens, server_port, grpc_port):
    os.chdir("../" + get_build_dir())
    cmd_string = f"./rubixgoplatform generatetestrbt -did {did_id} -numTokens {numTokens} -port {server_port} -grpcPort {grpc_port}"
    if is_windows_os():
        cmd_string = f".\\rubixgoplatform generatetestrbt -did {did_id} -numTokens {numTokens} -port {server_port} -grpcPort {grpc_port}"
    output, code = run_command(cmd_string, True)
    
    if code != 0:
        raise Exception("Error occurred while run the command: " + cmd_string)

    os.chdir("../tests")
    return output

def cmd_add_quorum_dids(server_port, grpc_port):
    os.chdir("../" + get_build_dir())
    cmd_string = f"./rubixgoplatform addquorum -port {server_port} -grpcPort {grpc_port}"
    if is_windows_os():
        cmd_string = f".\\rubixgoplatform addquorum -port {server_port} -grpcPort {grpc_port}"
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

def cmd_setup_quorum_dids(did, server_port, grpc_port):
    os.chdir("../" + get_build_dir())
    cmd_string = f"./rubixgoplatform setupquorum -did {did} -port {server_port} -grpcPort {grpc_port}"
    if is_windows_os():
        cmd_string = f".\\rubixgoplatform setupquorum -did {did} -port {server_port} -grpcPort {grpc_port}"
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

def check_account_info(did, server_port, grpc_port):
    os.chdir("../" + get_build_dir())
    cmd_string = f"./rubixgoplatform getaccountinfo -did {did} -port {server_port} -grpcPort {grpc_port}"
    if is_windows_os():
        cmd_string = f".\\rubixgoplatform getaccountinfo -did {did} -port {server_port} -grpcPort {grpc_port}"
    output, code = run_command(cmd_string)

    if code != 0:
        raise Exception("Error occurred while run the command: " + cmd_string)
    os.chdir("../tests")
    return output

# Note: address != did, address = peerId.didId 
def cmd_rbt_transfer(sender_address, receiver_address, rbt_amount, server_port, grpc_port):
    os.chdir("../" + get_build_dir())
    cmd_string = f"./rubixgoplatform transferrbt -senderAddr {sender_address} -receiverAddr {receiver_address} -rbtAmount {rbt_amount} -port {server_port} -grpcPort {grpc_port}"
    if is_windows_os():
        cmd_string = f".\\rubixgoplatform transferrbt -senderAddr {sender_address} -receiverAddr {receiver_address} -rbtAmount {rbt_amount} -port {server_port} -grpcPort {grpc_port}"
    output, code = run_command(cmd_string, True)
    print(output)
    if code != 0:
        raise Exception("Error occurred while run the command: " + cmd_string)

    os.chdir("../tests")
    return output