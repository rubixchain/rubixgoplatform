import subprocess
import os
import re

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
            return output, 1
        else:
            return output, code

def cmd_run_rubix_servers(node_name, server_port_idx, grpc_port):
    os.chdir("../linux")
    cmd_string = f"screen -S {node_name} -d -m ./rubixgoplatform run -p {node_name} -n {server_port_idx} -s -testNet -grpcPort {grpc_port}"
    _, code = run_command(cmd_string)
    if code != 0:
        raise Exception("Error occurred while run the command: " + cmd_string)
    os.chdir("../tests")

def cmd_create_did(server_port, grpc_port):
    os.chdir("../linux")
    cmd_string = f"./rubixgoplatform createdid -port {server_port} -grpcPort {grpc_port}"
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
    os.chdir("../linux")
    cmd_string = f"./rubixgoplatform registerdid -did {did_id} -port {server_port} -grpcPort {grpc_port}"
    output, code = run_command(cmd_string)
    print(output)

    if code != 0:
        raise Exception("Error occurred while run the command: " + cmd_string)

    os.chdir("../tests")
    return output

def cmd_generate_rbt(did_id, numTokens, server_port, grpc_port):
    os.chdir("../linux")
    cmd_string = f"./rubixgoplatform generatetestrbt -did {did_id} -numTokens {numTokens} -port {server_port} -grpcPort {grpc_port}"
    output, code = run_command(cmd_string, True)
    
    if code != 0:
        raise Exception("Error occurred while run the command: " + cmd_string)

    os.chdir("../tests")
    return output

def cmd_add_quorum_dids(server_port, grpc_port):
    os.chdir("../linux")
    cmd_string = f"./rubixgoplatform addquorum -port {server_port} -grpcPort {grpc_port}"
    output, code = run_command(cmd_string, True)
    print(output)
    if code != 0:
        raise Exception("Error occurred while run the command: " + cmd_string)

    os.chdir("../tests")
    return output

def cmd_shutdown_node(server_port, grpc_port):
    os.chdir("../linux")
    cmd_string = f"./rubixgoplatform shutdown -port {server_port} -grpcPort {grpc_port}"
    output, _ = run_command(cmd_string, True)
    print(output)

    os.chdir("../tests")
    return output

def cmd_setup_quorum_dids(did, server_port, grpc_port):
    os.chdir("../linux")
    cmd_string = f"./rubixgoplatform setupquorum -did {did} -port {server_port} -grpcPort {grpc_port}"
    output, code = run_command(cmd_string, True)
    print(output)
    if code != 0:
        raise Exception("Error occurred while run the command: " + cmd_string)

    os.chdir("../tests")
    return output

def cmd_get_peer_id(server_port, grpc_port):
    os.chdir("../linux")
    cmd_string = f"./rubixgoplatform get-peer-id -port {server_port} -grpcPort {grpc_port}"
    output, code = run_command(cmd_string)

    if code != 0:
        raise Exception("Error occurred while run the command: " + cmd_string)
    os.chdir("../tests")
    return output

def check_account_info(did, server_port, grpc_port):
    os.chdir("../linux")
    cmd_string = f"./rubixgoplatform getaccountinfo -did {did} -port {server_port} -grpcPort {grpc_port}"
    output, code = run_command(cmd_string)

    if code != 0:
        raise Exception("Error occurred while run the command: " + cmd_string)
    os.chdir("../tests")
    return output

# Note: address != did, address = peerId.didId 
def cmd_rbt_transfer(sender_address, receiver_address, rbt_amount, server_port, grpc_port):
    os.chdir("../linux")
    cmd_string = f"./rubixgoplatform transferrbt -senderAddr {sender_address} -receiverAddr {receiver_address} -rbtAmount {rbt_amount} -port {server_port} -grpcPort {grpc_port}"
    output, code = run_command(cmd_string, True)
    print(output)
    if code != 0:
        raise Exception("Error occurred while run the command: " + cmd_string)

    os.chdir("../tests")
    return output