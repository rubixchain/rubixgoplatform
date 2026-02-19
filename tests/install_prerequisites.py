import os
from prerequisite import get_testnet_ipfs_swarm_key, get_os_info, download_ipfs_binary, copy_fixtures_to_build_dir
from node.commands import run_command
from constants import IPFS_KUBO_VERSION

if __name__=='__main__':
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