import platform
import os
import shutil
import requests
import argparse
import binascii
from node.commands import run_command
from node.quorum import run_quorum_nodes

from scenarios import (
    rbt_transfer,
    bip39_nlss_test
)

IPFS_KUBO_VERSION = "v0.19.0"


def generate_ipfs_swarm_key(build_name):
    try:
        key = os.urandom(32)
    except Exception as e:
        print("While trying to read random source:", e)
        return

    output = "/key/swarm/psk/1.0.0/\n/base16/\n" + binascii.hexlify(key).decode()

    directory = "./test_swarm_key"
    filename = f"{directory}/testswarm_{build_name}.key"

    if not os.path.exists(directory):
        os.makedirs(directory)

    with open(filename, "w") as file:
        file.write(output)

def get_os_info():
    os_name = platform.system()
    build_folder = ""

    if os_name == "Linux":
        build_folder = "linux"
    elif os_name == "Windows":
        build_folder = "windows"
    elif os_name == "Darwin":
        build_folder = "mac"
    else:
        print("Unsupported operating system to build Rubix")
        return None, None

    return os_name, build_folder

def download_ipfs_binary(os_name, version, build_dir):
    download_url = ""
    
    if os_name == "Linux":
        download_url = f"https://dist.ipfs.tech/kubo/{version}/kubo_{version}_linux-amd64.tar.gz"
    elif os_name == "Windows":
        download_url = f"https://dist.ipfs.tech/kubo/{version}/kubo_{version}_windows-amd64.zip"
    elif os_name == "Darwin":  # MacOS
        download_url = f"https://dist.ipfs.tech/kubo/{version}/kubo_{version}_darwin-amd64.tar.gz"
    else:
        raise ValueError("Unsupported operating system")

    # Download the IPFS binary archive
    download_path = f"kubo_{version}_{os_name.lower()}-amd64.tar.gz" if os_name != "Windows" else f"kubo_{version}_{os_name.lower()}-amd64.zip"
    print("Downloading IPFS binary...")
    response = requests.get(download_url)
    with open(download_path, "wb") as f:
        f.write(response.content)
    print("Download completed.")

    # Extract the archive
    print("Extracting IPFS binary...")
    if os_name == "Windows":
        # For Windows, we need to use the 'zipfile' module to extract
        import zipfile
        with zipfile.ZipFile(download_path, "r") as zip_ref:
            zip_ref.extractall("kubo")
    else:
        # For Linux and MacOS, we use tar
        import tarfile
        with tarfile.open(download_path, "r:gz" if os_name != "Darwin" else "r") as tar_ref:
            tar_ref.extractall("kubo")
    print("Extraction completed.")

    # Check the contents of the kubo directory
    print("Contents of kubo directory:")
    for item in os.listdir("kubo"):
        print(item)

    # Move IPFS binary to the appropriate folder
    print("Moving IPFS binary...")
    
    ipfs_bin_name = "ipfs"
    if os_name == "Windows":
        ipfs_bin_name = "ipfs.exe"

    src_file = os.path.join("kubo", "kubo", ipfs_bin_name)
    dest_dir = os.path.join(build_dir, ipfs_bin_name)
    if os.path.exists(src_file):
        shutil.move(src_file, dest_dir)
        print("IPFS binary moved to", dest_dir)

        # Check if the file is present at the destination
        dest_file = os.path.join(dest_dir)
        if not os.path.exists(dest_file):
            raise FileNotFoundError("IPFS binary not found at the destination after move operation.")
    else:
        raise FileNotFoundError("Installed IPFS binary file does not exist.")

    # Clean up
    os.remove(download_path)
    shutil.rmtree("kubo")
    print("\nIPFS has been installed succesfully.")

def copy_fixtures_to_build_dir(build_directory):
    fixtures_directory = os.path.join("tests", "fixtures")
    
    # Copy didimage.png.file
    image_file_src = os.path.join(fixtures_directory, "didimage.png.file")
    image_file_dest = os.path.join(build_directory, "image.png")
    shutil.copyfile(image_file_src, image_file_dest)
    
    if not os.path.exists(image_file_dest):
        raise FileNotFoundError(f"Copy operation for didimage.png.file failed. Destination file not found: {image_file_dest}")
    
    # Copy testswarm.key
    swarm_key_dir = os.path.join("tests", "test_swarm_key")
    swarmkey_src = os.path.join(swarm_key_dir, f"testswarm_{build_directory}.key")
    swarmkey_dest = os.path.join(build_directory, f"testswarm.key")
    shutil.copyfile(swarmkey_src, swarmkey_dest)

    if not os.path.exists(swarmkey_dest):
        raise FileNotFoundError(f"Copy operation for testswarm_{build_directory}.key failed. Destination file not found: {swarmkey_dest}")

    print("\nimage.png and swarm key have been added to build directory successfully")

def cli():
    parser = argparse.ArgumentParser(description="CLI to run tests for Rubix")
    parser.add_argument("--skip_prerequisite", action=argparse.BooleanOptionalAction, help="skip prerequisite steps such as installing IPFS and building Rubix")
    parser.add_argument("--run_nodes_only", action=argparse.BooleanOptionalAction, help="only run the rubix nodes and skip the setup")
    parser.add_argument("--skip_adding_quorums", action=argparse.BooleanOptionalAction, help="skips adding quorums")
    parser.add_argument("--run_tests_only", action=argparse.BooleanOptionalAction, help="only proceed with running tests")
    return parser.parse_args()

if __name__=='__main__':
    args = cli()
    skip_prerequisite = args.skip_prerequisite
    run_nodes_only = args.run_nodes_only
    skip_adding_quorums = args.skip_adding_quorums
    run_tests_only = args.run_tests_only

    non_quorum_node_config = {}

    if not run_tests_only:
        os_name, build_folder = get_os_info()
        if os_name is None:
            exit(1)

        generate_ipfs_swarm_key(build_folder)

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

        if not skip_prerequisite:
            download_ipfs_binary(os_name, IPFS_KUBO_VERSION, build_folder)
            copy_fixtures_to_build_dir(build_folder)
       
        os.chdir("./tests")
        
        run_quorum_nodes(run_nodes_only, skip_adding_quorums=skip_adding_quorums)
    
    # It will carry list of Python files containing the function `run()`
    # that consists of logic to run all the necessary tests
    modules = [
        rbt_transfer,
        bip39_nlss_test,
    ]

    for module in modules:
        module.run()
