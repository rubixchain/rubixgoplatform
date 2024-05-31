# Rubix Blockchain Platform

Rubix is a highly scalable, zero-free blockchain that overcomes the scale, cost, and privacy issues of traditional sequentially organized blockchains. It utilizes a novel Proof-of-Pledge (PoP) consensus mechanism, designed to address the drawbacks of Proof-of-Work (PoW) and Proof-of-Stake (PoS) systems.

### Building from Source

1. Clone the repository

    ```
    git clone https://github.com/rubixchain/rubixgoplatform.git
    cd rubixgoplatform
    ```

2. Based on the OS, run any one of the following commands to build `rubixgoplatform` binary:

- Linux (binary will be created in `linux` directory)

    ```
    make compile-linux
    ```

- MacOS (binary will be created in `mac` directory)

    ```
    make compile-mac
    ```

- Windows (binary will be created in `windows` directory)

    ```
    make compile-windows
    ```

## Running a Rubix Node

0. Build `rubixgoplaform` (Refer previous section)

1. Install IPFS Kubo Client (version: `0.21.0`) and copy the IPFS binary to build directory. Refer the section based on your Operating System 

    <details>
    <summary>Windows Installation</summary>
        
    - In Powershell, run the following to install the IPFS kubo client:

    ```
    wget https://dist.ipfs.tech/kubo/v0.21.0/kubo_v0.21.0_windows-amd64.zip -Outfile kubo_v0.21.0.zip
    ```
    
    - Extract `kubo_v0.21.0.zip`

    ```
    Expand-Archive -Path kubo_v0.28.0.zip
    ```

    - Copy the `ipfs` binary to build directory

    ```
    cp .\kubo_v0.28.0\kubo\ipfs.exe <path-to-rubixgoplatform>\windows\
    ```
    </details>

    <details>
    <summary>Linux Installation</summary>
        
    - Run the following to install the IPFS kubo client:

    ```
    wget https://dist.ipfs.tech/kubo/v0.21.0/kubo_v0.21.0_linux-amd64.tar.gz
    ```
    
    - Extract `kubo_v0.21.0_linux-amd64.tar.gz`

    ```
    tar -xvzf kubo_v0.21.0_linux-amd64.tar.gz
    ```

    - Copy the `ipfs` binary to build directory

    ```
    cp kubo/ipfs <path-to-rubixgoplatform>/linux/
    ```
    </details>

    <details>
    <summary>MacOS Installation</summary>
        
    - Run the following to install the IPFS kubo client:

    ```
    wget https://dist.ipfs.tech/kubo/v0.21.0/kubo_v0.21.0_darwin-arm64.tar.gz
    ```
    
    - Extract `kubo_v0.21.0_darwin-arm64.tar.gz`

    ```
    tar -xvzf kubo_v0.21.0_darwin-arm64.tar.gz
    ```

    - Copy the `ipfs` binary to build directory

    ```
    cp kubo/ipfs <path-to-rubixgoplatform>/mac/
    ```
    </details>

2. Copy the `swarm.key` present in the root directory into the build directory and rename it to `testswarm.key`.

3. Run the following from the build directory:

    - Linux and Macos

    ```
    ./rubixgoplatform run --p node0 --n 0 --s --testNet --grpcPort 10500
    ```

    - Windows

    ```
    .\rubixgoplatform.exe run --p node0 --n 0 --s --testNet --grpcPort 10500
    ```

    The above command creates a `node0` name directory inside the build directory which hosts the configuration and DB files.The `rubixgodaemon` daemon runs on port `20000` and the gRPC server runs on `10500`

## Rubix CLI

Please refer [Rubix CLI](./command/README.md) docs for more details on the `rubixgoplatform` CLI
