<p align="center">
  <img src="Rubix_logo.png" alt="Your Organization Logo" width="300"/>
</p>

<h1 align="center">RUBIX NETWORKING</h1>

<p align="center">
  <b>An L1 monolithic blockchain protocol</b>
</p>

---
# Rubix Go Platform

Welcome to the RubixGoPlatform !!!
This README provides comprehensive documentation of the platform’s command-line interface (CLI) tools alongside RESTful API endpoints, enabling developers and operators to easily interact with the Rubix blockchain node and its services.

## Table of Contents

- [Overview](#overview)
- [Prerequisites](#prerequisites)
- [Quick Setup](#quick-setup)
- [CLI Commands](#cli-commands)
  - [Basic Commands](#basic-commands)
  - [Node Management](#node-management)
  - [Bootstrap Management](#bootstrap-management)
  - [DID Management](#did-management)
  - [Quorum Management](#quorum-management)
  - [Token Management](#token-management)
  - [Token Chain Operations](#token-chain-operations)
  - [FT Operations](#ft-operations)
  - [NFT Operations](#nft-operations)
  - [Smart Contract Operations](#smart-contract-operations)
  - [Explorer Management](#explorer-management)
  - [Peer Management](#peer-management)
  - [Token Status & Monitoring](#token-status--monitoring)
  - [Transaction Management](#transaction-management)
  - [Migration & Recovery](#migration--recovery)
- [Support and Community](#support-and-community)
- [Version Compatibility](#version-compatibility)

---

## Overview

The **Rubix Go Platform** provides a high-performance blockchain node for the Rubix network, offering a powerful CLI to manage nodes, peers, and bootstrap configurations. Built with Go, it ensures reliability and scalability for decentralized applications and network operations.

---

## Prerequisites

To run a **Rubix node**, you can use the latest [release binary](https://github.com/rubixchain/rubixgoplatform/releases) or build from source. The following tools are required for building from source:

| Tool                      | Version       | Purpose                                              |
|---------------------------|---------------|------------------------------------------------------|
| **Go**                    | 1.20+         | Build the `rubixgoplatform` binary via `make`        |
| **Make**                  | Latest        | Automates the build process                          |
| **IPFS Binary**           | v0.21.0       | Enables decentralized storage                        |
| **Git**                   | Latest        | Clones the Rubix repository                          |
| **Test Swarm Key**        | Latest        | (`testswarm.key`) Required to join the Rubix testnet |
| **Swarm Key**             | Latest        | (`swarm.key`) Required to join the Rubix Mainnet     |


---

## Quick Setup

To build and run a Rubix node from source:

### Clone the repository

```bash
git clone https://github.com/rubixchain/rubixgoplatform.git
cd rubixgoplatform
```

### Build the binary

```bash
# For mac:
 make compile-mac

# For linux:
 make compile-linux

# For windows:
 make compile-windows

# Remove existing binary:
 make clean
```
- Find the binary file in respective OS folder

- Add the IPFS binary file to the respective OS folder

- Move the `testswarm.key`/`swarm.key` from root folder(rubixgoplatform) to respective OS folder

---

## CLI Commands

### Basic Commands

#### Version

Display the version of the Rubix Go Platform executable.

```bash
./rubixgoplatform -v
```

**Description:** Retrieves the current version of the Rubix Go Platform.

**Options:**
- `-v`: Display the executable version.

---

#### Help

Display help information for available CLI commands and options.

```bash
./rubixgoplatform -h
```

**Description:** Shows a list of available commands and their usage.

**Options:**
- `-h`: Show help information of commands.

---

### Node Management

#### Run Node

Start a Rubix blockchain node.

```bash
./rubixgoplatform run -p <node-name> -n <node-number> -s -testNet
```

**Description:** Initializes and runs the Rubix node, enabling core blockchain functionality.

**Options:**
- `-n uint`: Node number.
- `-p string`: Working directory path (default: "./").
- `-s`: Start the core.
- `-testNet`: Run as test network.
- `-testNetKey string`: Test network key (default: "testswarm.key").

**Related API:** `POST /api/start`

---

#### Shutdown Node

Shut down the Rubix node.

```bash
./rubixgoplatform shutdown -port <port-number>
```

**Description:** Gracefully terminates the Rubix node.

**Options:**
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/shutdown`

---

#### Get Peer ID

Retrieve the node's peer ID.

```bash
./rubixgoplatform get-peer-id -port <port-number>
```

**Description:** Fetches the unique peer ID of the running node.

**Options:**
- `-port string`: Server/Host port (default: "20000").

**Related API:** `GET /api/get-peer-id`

---

#### Ping Peer

Test connectivity with a specific peer in the Rubix network.

```bash
./rubixgoplatform ping -peerID <peer-id> -port <port-number>
```

**Description:** Pings a peer to verify network connectivity.

**Options:**
- `-addr string`: Server/Host address (default: "localhost").
- `-peerID string`: Peer ID.
- `-port string`: Server/Host port (default: "20000").

**Related API:** `GET /api/ping`

---

### Bootstrap Management

#### Add Bootstrap

Add bootstrap peers to the Rubix node.

```bash
./rubixgoplatform add-bootstrap -peers <bootstarp-peer-address> -port <port-number>
```

**Description:** Adds bootstrap peers to facilitate network discovery and connectivity.

**Options:**
- `-addr string`: Server/Host address (default: "localhost").
- `-peers string`: Bootstrap peers (comma-separated for multiple peers).
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/add-bootstrap`

---

#### Remove Bootstrap

Remove specific bootstrap peers from the node.

```bash
./rubixgoplatform remove-bootstrap -peers <bootstarp-peer-address> -port <port-number>
```

**Description:** Removes specified bootstrap peers from the node configuration.

**Options:**
- `-addr string`: Server/Host address (default: "localhost").
- `-peers string`: Bootstrap peers to remove (comma-separated).
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/remove-bootstrap`

---

#### Remove All Bootstrap

Remove all bootstrap peers from the node.

```bash
./rubixgoplatform remove-all-bootstrap -port <port-number>
```

**Description:** Clears all configured bootstrap peers.

**Options:**
- `-addr string`: Server/Host address (default: "localhost").
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/remove-all-bootstrap`

---

#### Get All Bootstrap

List all configured bootstrap peers.

```bash
./rubixgoplatform get-all-bootstrap -port <port-number>
```

**Description:** Retrieves a list of all bootstrap peers configured on the node.

**Options:**
- `-addr string`: Server/Host address (default: "localhost").
- `-port string`: Server/Host port (default: "20000").

**Related API:** `GET /api/get-all-bootstrap`

---

### DID Management

#### Create DID

Create a new Decentralized Identifier (DID).

```bash
./rubixgoplatform create-did -didType <did-type> -port <port-number>
```

**Description:** Generates a new DID for use in the Rubix network.

**Options:**
- `-port string`: Server/Host port (default: "20000").
- `-didType int`: DID type (0=Basic, 1=Standard, 2=Wallet, 3=Child, 4=Light; default: 0).
- `-didSecret string`: DID secret (default: "My DID Secret").
- `-privPWD string`: Private key password (default: "mypassword").
- `-quorumPWD string`: Quorum key password (default: "mypassword").
- `-imgFile string`: Image file for DID creation (must be 256x256 PNG; default: "image.png").
- `-didImgFile string`: DID image file name (default: "did.png").
- `-privImgFile string`: Private share image file name (default: "pvtShare.png").
- `-pubImgFile string`: Public share image file name (default: "pubShare.png").
- `-privKeyFile string`: Private key file name (default: "pvtKey.pem").
- `-pubKeyFile string`: Public key file name (default: "pubKey.pem").
- `-mnemonicKeyFile string`: Mnemonic key file (default: "mnemonic.txt").
- `-ChildPath int`: BIP Child Path (default: 0).
- `-fp`: Force password entry in terminal.

**Related API:** `POST /api/createdid`

---

#### Create DID from Public Key

Create a DID from an existing public key.

```bash
./rubixgoplatform create-did-from-pubkey
```

**Description:** Generates a DID using a provided public key.

**Related API:** `POST /api/request-did-for-pubkey`

---

#### Get All DIDs

List all DIDs on the node.

```bash
./rubixgoplatform get-all-did -port <port-number>
```

**Description:** Retrieves a list of all DIDs configured on the node.

**Options:**
- `-addr string`: Server/Host address (default: "localhost").
- `-port string`: Server/Host port (default: "20000").

**Related API:** `GET /api/getalldid`

---

#### Register DID

Register a DID and PeerID mapping on the network.

```bash
./rubixgoplatform register-did -did <did-address> -port <port-number>
```

**Description:** Registers a DID with its associated PeerID on the Rubix network.

**Options:**
- `-did string`: DID address.
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/register-did`

---

#### Setup DID

Configure DID settings.

```bash
./rubixgoplatform setup-did
```

**Description:** Sets up configuration for a DID on the node.

**Related API:** `POST /api/setup-did`

---

### Quorum Management

#### Add Quorum

Add a quorum list to the node.

```bash
./rubixgoplatform add-quorum -quorumList <quorumlist-json-file-name> -port <port-number>
```

**Description:** Configures a quorum list for the node.

**Options:**
- `-quorumList string`: Quorum list file name (default: "quorumlist.json").
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/addquorum`

---

#### Setup Quorum

Set up quorum with private key password.

```bash
./rubixgoplatform setup-quorum -did <did-address> -port <port-number>
```

**Description:** Configures quorum settings with specified passwords.

**Options:**
- `-did string`: DID address.
- `-port string`: Server/Host port (default: "20000").
- `-quorumPWD string`: Quorum key password (default: "mypassword").
- `-privPWD string`: Private key password (default: "mypassword").
- `-fp`: Force password entry in terminal.

**Related API:** `POST /api/setup-quorum`

---

#### Get All Quorum

List all quorum configurations.

```bash
./rubixgoplatform get-all-quorum -port <port-number>
```

**Description:** Retrieves all quorum configurations on the node.

**Options:**
- `-addr string`: Server/Host address (default: "localhost").
- `-port string`: Server/Host port (default: "20000").

**Related API:** `GET /api/getallquorum`

---

#### Remove All Quorum

Remove all quorum configurations.

```bash
./rubixgoplatform remove-all-quorum -port <port-number>
```

**Description:** Clears all quorum configurations from the node.

**Options:**
- `-addr string`: Server/Host address (default: "localhost").
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/removeallquorum`

---

#### Check Quorum Status

Check the status of quorum configuration.

```bash
./rubixgoplatform check-quorum-status -quorumAddr <quorum-did-address> -port <port-number>
```

**Description:** Verifies the status of the quorum configuration.

**Options:**
- `-addr string`: Server/Host address (default: "localhost").
- `-port string`: Server/Host port (default: "20000").
- `-quorumAddr string`: Quorum DID address.

**Related API:** `GET /api/check-quorum-status`

---

### Token Management

#### Generate Test RBT

Generate test RBT tokens on the node.

```bash
./rubixgoplatform generate-test-rbt -did <did-address> -numTokens <token-amount> -port <port-number>
```

**Description:** Creates test RBT tokens for a specified DID.

**Options:**
- `-did string`: DID address.
- `-numTokens int`: Number of tokens to generate (default: 1).
- `-port string`: Server/Host port (default: "20000").
- `-fp`: Force password entry.
- `-privPWD string`: Private key password (default: "mypassword").
- `-privImgFile string`: Private share image file (default: "pvtShare.png").
- `-privKeyFile string`: Private key file (default: "pvtKey.pem").

**Related API:** `POST /api/generate-test-token`

---

#### Generate Faucet Test RBT

Generate faucet test RBT tokens.

```bash
./rubixgoplatform generate-faucet-rbt
```

**Description:** Creates faucet test RBT tokens for testing purposes.

**Related API:** `POST /api/generate-faucettest-token`

---

#### Transfer RBT

Transfer RBT tokens between addresses.

```bash
./rubixgoplatform transfer-rbt -senderAddr <sender-did> -receiverAddr <receiver-did> -rbtAmount <token-amonunt> -port <port-number>
```

**Description:** Transfers RBT tokens from a sender to a receiver.

**Options:**
- `-senderAddr string`: Sender address.
- `-receiverAddr string`: Receiver address.
- `-rbtAmount float`: RBT amount to transfer.
- `-port string`: Server/Host port (default: "20000").
- `-transComment string`: Transfer comment (default: "Test transaction").
- `-transType int`: Transaction type (default: 2).
- `-fp`: Force password entry.
- `-privPWD string`: Private key password.
- `-privImgFile string`: Private share image file.
- `-privKeyFile string`: Private key file.

**Related API:** `POST /api/initiate-rbt-transfer`

---

<!-- #### Self Transfer RBT

Perform self-transfer of RBT tokens.

```bash
./rubixgoplatform self-transfer-rbt -did <did-address> -amount <token-amount> -port <port-number>
```

**Description:** Transfers RBT tokens to the same DID.

**Related API:** `POST /api/initiate-self-transfer`

--- -->

#### Get Account Info

Retrieve account information for a DID.

```bash
./rubixgoplatform get-account-info -did <did-address> -port <port-number>
```

**Description:** Fetches account details for a specified DID.

**Options:**
- `-did string`: DID address.
- `-port string`: Server/Host port (default: "20000").

**Related API:** `GET /api/get-account-info`

---

<!-- #### Lock Tokens

Lock tokens for specific operations.

```bash
./rubixgoplatform lock-tokens
```

**Description:** Locks tokens to restrict their use in certain operations.

**Related API:** `POST /api/lock-tokens`

--- -->

<!-- #### Release All Locked Tokens

Release all locked tokens.

```bash
./rubixgoplatform release-all-locked-tokens
```

**Description:** Unlocks all previously locked tokens.

**Related API:** `POST /api/release-all-locked-tokens`

--- -->

#### Pin Token

Pin RBT tokens for pinning service.

```bash
./rubixgoplatform pin-token -pinningAddress <pinning-node-did-address> -senderAddr <sender-address> -rbtAmount <token-amount> -port <port-number>
```

**Description:** Pins tokens to ensure secure handling.

**Options:**
- `-pinningAddress string`: Pinning DID address.
- `-senderAddr string`: Sender address.
- `-rbtAmount float`: RBT token amount.
- `-transComment string`: Transfer comment (default: "Test transaction").
- `-transType int`: Transaction type (default: 2).
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/initiate-pin-token`

---

#### Recover Tokens

Recover RBT tokens after pinning for pinning service.

```bash
./rubixgoplatform recover-token -pinningAddress <pinning-node-did-address> -senderAddr <sender-address> -rbtAmount <token-amount> -port <port-number>
```

**Description:** Recovers lost or inaccessible RBT tokens.

**Options:**
- `-pinningAddress string`: Pinning DID address.
- `-senderAddr string`: Sender address.
- `-rbtAmount float`: RBT token amount.
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/recover-token`

---

#### Validate Token

Validate a specific token.

```bash
./rubixgoplatform validatetoken -token <token-ID> -port <port-number>
```

**Description:** Verifies the validity of a specific token.

**Options:**
- `-token string`: Token ID.
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/validate-token`

---

#### Faucet Token Check

Check faucet token status.

```bash
./rubixgoplatform faucet-token-check -did <did-address> -token <token-ID> -port <port-number>
```

**Description:** Retrieves the status of faucet tokens.

**Options:**
- `-token string`: Token ID.
- `-did string`: DID address.
- `-port string`: Server/Host port (default: "20000").

**Related API:** `GET /api/faucet-token-check`

---

### Token Chain Operations

#### Dump Token Chain

Get RBT token chain data.

```bash
./rubixgoplatform dump-tokenchain -token <token-id> -port <port-number>
```

**Description:** Get data from a RBT token chain to a json file.

**Options:**
- `-token string`: Token ID.
- `-port string`: Server/Host port (default: "20000").

**Note:**
- The token chain data will be saved as a json file named `dump.json` in the current directory.

**Related API:** `GET /api/dump-token-chain`

---

#### Decode Token Chain

Decode dumped token chain data.

```bash
./rubixgoplatform decode-tokenchain
```

**Description:** Decodes previously dumped token chain (`dump.json`) data for better readability.

---

#### Validate Token Chain

Validate RBT and smart contract token chains.

```bash
./rubixgoplatform validate-tokenchain -did <did-address> -token <token-ID> -port <port-number>
```

**Description:** Validates the integrity of token chains.

**Options:**
- `-did string`: DID address.
- `-sctValidation bool`: Enable smart contract validation (default: false).
- `-token string`: Token ID.
- `-allmyTokens bool`: Validate all tokens (default: false).
- `-blockCount int`: Number of blocks to validate (default: 0, all blocks).

**Related API:** `POST /api/validate-token-chain`

---

### FT Operations

#### Create FT

Create fungible tokens.

```bash
./rubixgoplatform create-ft -did <did-address> -ftName <name/symb-for-ft> -ftCount <token-amount> -rbtAmount <token-amount> -port <port-number>
```

**Description:** Generates FTs on the node.

**Options:**
- `-did string`: DID address.
- `-ftName string`: Name of the FT.
- `-ftCount int`: Number of FTs to create.
- `-rbtAmount int`: RBT amount for FT creation.
- `-port string`: Server/Host port (default: "20000").
- `-ftStartIndex int`: FT number start index (default: "0").

**Note:**
- Use the `-ftStartIndex` flag when creating additional FTs with the same DID address and FT name. This ensures numbering continues from the correct index.

**Related API:** `POST /api/create-ft`

---

#### Transfer FT

Transfer fungible tokens.

```bash
./rubixgoplatform transfer-ft -ftName <name/symb-of-ft> -ftCount <token-amount> -senderAddr <sender-did> -receiverAddr <receiver-did> -port <port-number>
```

**Description:** Transfers FTs between addresses.

**Options:**
- `-ftName string`: FT name to transfer.
- `-ftCount int`: Number of FTs to transfer.
- `-senderAddr string`: Sender address.
- `-receiverAddr string`: Receiver address.
- `-port string`: Server/Host port (default: "20000").
- `-transType int`: Transaction type (default: 2).
- `-fp`: Force password authentication.
- `-creatorDID string`: FT Creator DID (for multiple FTs with the same name).

**Note:**
- Use the `-creatorDID` flag when transferring FTs with duplicate names created by different DIDs, to accurately identify and transfer the correct FT batch. 

**Related API:** `POST /api/initiate-ft-transfer`

---

#### Get FT Info by DID

Retrieve information about all FTs for a DID.

```bash
./rubixgoplatform get-ft-info-by-did -did <did-address> -port <port-number>
```

**Description:** Fetches FT balance associated with a DID.

**Options:**
- `-did string`: DID address.
- `-port string`: Server/Host port (default: "20000").

**Related API:** `GET /api/get-ft-info-by-did`

---

#### Dump FT Token Chain

Get FT token chain data.

```bash
./rubixgoplatform dump-ft -token <ft-token-id> -port <port-number>
```

**Description:** Get data from an FT token chain to a json file.

**Options:**
- `-token string`: FT token ID.
- `-port string`: Server/Host port (default: "20000").

**Related API:** `GET /api/dump-ft-token-chain`

**Note:**
- The token chain data will be saved as a json file named `dump.json` in the current directory.

---

#### Get FT Transaction Details

Retrieve FT transaction details by DID.

```bash
./rubixgoplatform get-ft-txn-details -did <did-address> -port <port-number>
```

**Description:** Fetches transaction details for FTs associated with a DID.

**Options:**
- `-did string`: DID address.
- `-port string`: Server/Host port (default: "20000").

**Related API:** `GET /api/get-ft-txn-by-did`

---













### NFT Operations

#### Create NFT

Create Non-Fungible Tokens (NFTs).

```bash
./rubixgoplatform create-nft -did <did-address> -metadata <file-path> -artifact <file-path> -port <port-number>
```

**Description:** Generates new NFTs on the node.

**Options:**
- `-did string`: DID address.
- `-port string`: Server/Host port (default: "20000").
- `-metadata string`: Path of JSON file which contains information about the NFT (default: "")
- `-artifact string`: Path of file which is meant to be an NFT (default: "")

**Related API:** `POST /api/create-nft`

---

#### Deploy NFT

Deploy an NFT smart contract.

```bash
./rubixgoplatform deploy-nft -deployerAddr <deployer-did-address> -nft <nft-ID> -nftValue <value-of-nft> -port <port-number>
```

**Description:** Deploys an NFT smart contract to the network.

**Options:**
- `-nft string`: NFT Id (default "")
- `-deployerAddr string`: DID address of deployer of the NFT.
- `-transType int`: Quorum type (default: "2").
- `-nftValue float`: Value of the NFT (default: "0.0")
- `-nftData string`: Arbitrary data associated with NFT (default "").
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/deploy-nft`

---

#### Execute NFT

Execute NFT transactions.

```bash
./rubixgoplatform execute-nft -nft <nft-ID> -owner <owner-did-address> -receiver <receiver-address> -rbtAmount <token-amount> -nftData <arbitrary-data> -port <port-number>
```

**Description:** Performs NFT transactions.

**Options:**
- `-nft string`: NFT Id (default "").
- `-executorAddr string`: DID address of the executor of the NFT (default "").
- `-receiver string`: DID that receives the ownership of NFT (default "").
- `-transType int`: Quorum type (default: "2").
- `-transComment string`: Transaction Comment (default "").
- `-rbtAmount float`: Sale value of NFT (default "0.0").
- `-nftData string`: Arbitrary data associated with NFT (default "").
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/execute-nft`

---

#### Get All NFT

List all NFTs.

```bash
./rubixgoplatform get-all-nft -did <did-address> -port <port-number>
```

**Description:** Retrieves a list of all NFTs on the node.

**Options:**
- `-did string`: DID address.

**Related API:** `GET /api/list-nfts`

---

#### Subscribe NFT

Subscribe to NFT events.

```bash
./rubixgoplatform subscribe-nft -nft <nft-ID> -port <port-number>
```

**Description:** Subscribes to NFT token chain updates.

**Options:**
- `-nft string`: NFT Id (default "").

**Related API:** `POST /api/subscribe-nft`

---

#### Fetch NFT

Fetch NFT details.

```bash
./rubixgoplatform fetch-nft -nft <nft-ID> -port <port-number>
```

**Description:** Retrieves detailed information of NFT from network to the node

**Options:**
- `-nft string`: NFT Id (default "").

**Related API:** `GET /api/fetch-nft`

---

#### Get NFTs by DID

Retrieve NFTs owned by a specific DID.

```bash
./rubixgoplatform get-nfts-by-did -did <did-address> -port <port-number>
```

**Description:** Lists all NFTs associated with a given DID.

**Options:**
- `-did string`: DID address.

**Related API:** `GET /api/get-nfts-by-did`

---

#### Dump NFT Token Chain

Export NFT token chain data.

```bash
./rubixgoplatform dump-nft-tokenchain -nft <nft-ID> -port <port-number> 
```

**Description:** Exports data from an NFT token chain to a json file.

**Options:**
- `-nft string`: NFT Id (default "").

**Related API:** `GET /api/dump-nft-token-chain`

---

### Smart Contract Operations

#### Generate Smart Contract

Generate smart contract code.

```bash
./rubixgoplatform generate-sct -did <did-address> -binCode <binary-code-file-path> -rawCode <raw-code-file-path> -schemaFile <schema-file-path> -port <port-number>
```

**Description:** Creates smart contract token for deployment.

**Options:**
- `-did string`: DID address.
- `-port string`: Server/Host port (default: "20000").
- `-binCode string`: Path of wasm file which is compiled from raw contract (default: "").
- `-rawCode string`: Path of raw smart contract code file (default: "")
- `-schemaFile string`: Path of json file which can be used to track state change (default: "").

**Related API:** `POST /api/generate-smart-contract`

---

#### Deploy Smart Contract

Deploy a smart contract to the network.

```bash
./rubixgoplatform deploy-smartcontract -sct <smartcontract-ID> -deployerAddr <deployer-did-address> -rbtAmount <token-amount> -transType <quorum-type> -transComment <transaction-comment> -port <port-number>
```

**Description:** Deploys a smart contract to the Rubix network.

**Options:**
- `-sct string`: Smart contract ID (default "").
- `-deployerAddr string`: DID address of deployer of the smart contract.
- `-rbtAmount float`: Value of the smart contract (default: "0.0").
- `-transType int`: Quorum type (default: "2").
- `-transComment string`: Transaction comment (default "").
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/deploy-smart-contract`

---

#### Execute Smart Contract

Execute smart contract functions.

```bash
./rubixgoplatform execute-smartcontract -sct <smartcontract-ID> -executorAddr <executor-did-address> -transType <quorum-type> -transComment <transaction-comment> -sctData <execution-input-data> -port <port-number>
```

**Description:** Runs specified functions within a smart contract.

**Options:**
- `-sct string`: Smart contract ID (default "").
- `-executorAddr string`: DID address of executor of the smart contract.
- `-transType int`: Quorum type (default: "2").
- `-transComment string`: Transaction comment (default "").
- `-sctData string`: Arbitrary data associated with smart contract (default "").
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/execute-smart-contract`

---

#### Fetch Smart Contract

Fetch smart contract details.

```bash
./rubixgoplatform fetch-sct -sct <smartcontract-ID> -port <port-number>
```

**Description:** Retrieves details of a smart contract.

**Options:**
- `-sct string`: Smart contract ID (default "").
- `-port string`: Server/Host port (default: "20000").

**Related API:** `GET /api/fetch-smart-contract`

---

#### Publish Smart Contract

Publish a smart contract to the network.

```bash
./rubixgoplatform publish-sct -sct <smartcontarct-ID> -did <publisher-did-address> -pubType <publishing-event-type> -sctBlockHash <publishing-block-hash> -port <port-number>
```

**Description:** Publishes a smart contract for network-wide access.

**Options:**
- `-sct string`: Smart contract ID.
- `-did string`: Publisher DID address.
- `-pubType int`: Smart contract event publishing type- Deploy:1 & Execute:2 (Default: "0").
- `-sctBlockHash string`: Smart contract block.
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/publish-smart-contract`

---

#### Subscribe Smart Contract

Subscribe to smart contract events.

```bash
./rubixgoplatform subscribe-sct -sct <smartcontract-ID> -port <port-number>
```

**Description:** Subscribes to events emitted by a smart contract.

**Options:**
- `-sct string`: Smart contract ID
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/subscribe-smart-contract`

---

#### Dump Smart Contract Token Chain

Export smart contract token chain data.

```bash
./rubixgoplatform dump-smartcontract-tokenchain -sct <smartcontract-ID> -port <port-number>
```

**Description:** Save data from a smart contract token chain to json file.

**Options:** 
- `-sct string`: Smartcontract ID
- `-port string`: Server/Host port (default: "20000").

**Related API:** `GET /api/dump-smart-contract-token-chain`

<!-- ---

#### Get Smart Contract Data

Retrieve smart contract token chain data.

```bash
./rubixgoplatform get-smartcontract-data -sct <smartcontract-ID> -port <port-number>
```

**Description:** Fetches data from a smart contract token chain.

**Options:** 
- `-sct string`: Smartcontract ID
- `-port string`: Server/Host port (default: "20000").

**Related API:** `GET /api/get-smart-contract-token-chain-data`

--- -->

<!-- ### Data Token Operations

#### Create Data Token

Create data tokens for data storage.

```bash
./rubixgoplatform create-datatoken
```

**Description:** Generates data tokens for storing data on the network.

**Related API:** `POST /api/create-data-token`

---

#### Commit Data Token

Commit data token to the network.

```bash
./rubixgoplatform commit-datatoken
```

**Description:** Commits a data token to the Rubix network.

**Related API:** `POST /api/commit-data-token`

--- -->

<!-- ### Service Management

#### Setup Service

Configure services on the node.

```bash
./rubixgoplatform setup-service -srvName explorer_service
```

**Description:** Configures services running on the node.

**Options:**
- `-srvName string`: Service name (default: "explorer_service").
- `-dbAddress string`: Database address (default: "localhost").
- `-dbName string`: Database name (default: "ExplorerDB").
- `-dbPassword string`: Database password (default: "password").
- `-dbPort string`: Database port (default: "1433").
- `-dbType string`: Database type (SQLServer, PostgreSQL, MySQL, Sqlite3; default: "SQLServer").
- `-dbUsername string`: Database username (default: "sa").

**Related API:** `POST /api/setup-service`

---

#### Setup Database

Configure database settings.

```bash
./rubixgoplatform setup-db
```

**Description:** Sets up database configuration for the node.

**Related API:** `POST /api/setup-db`

---

#### Update Configuration

Update node configuration.

```bash
./rubixgoplatform update-config
```

**Description:** Updates the node's configuration settings.

--- -->

### Explorer Management

#### Add Explorer

Add explorer URLs for transaction data.

```bash
./rubixgoplatform add-explorer -links <url> -port <port-number>
```

**Description:** Adds explorer URLs to access transaction data.

**Options:**
- `-links string`: URLs to add (comma-separated for multiple).
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/add-explorer`

---

#### Remove Explorer

Remove explorer URLs.

```bash
./rubixgoplatform remove-explorer -links <url> -port <port-number>
```

**Description:** Removes specified explorer URLs.

**Options:**
- `-links string`: URLs to remove (comma-separated for multiple).
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/remove-explorer`

---

#### Get All Explorer

List all configured explorer URLs.

```bash
./rubixgoplatform get-all-explorer -port <port-number>
```

**Description:** Retrieves a list of all configured explorer URLs.

**Options:**
- `-port string`: Server/Host port (default: "20000").

**Related API:** `GET /api/get-all-explorer`

---

#### Add User API Key

Add an API key for user authentication.

```bash
./rubixgoplatform add-user-apikey -did <did-address> -apiKey <api-key> -port <port-number> 
```

**Description:** Adds an API key for authenticating users.

**Options:**
- `-did string`: DID address.
- `-port string`: Server/Host port (default: "20000").
- `-apiKey string`: API-Key corresponding to DID.

**Related API:** `POST /api/add-user-api-key`

---

### Peer Management

#### Add Peer Details

Manually add peer details.

```bash
./rubixgoplatform add-peer-details -peerID <peer-id> -did <did-address> -didType <did-type> -port <port-number>
```

**Description:** Adds peer details to the node configuration.

**Options:**
- `-peerID string`: Peer ID.
- `-did string`: DID address.
- `-didType int`: DID type (0=Basic, 1=Standard, 2=Wallet, 3=Child, 4=Light; default: 0).
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/add-peer-details`

---

#### Add Peer Details from Explorer

Add peer details from explorer data.

```bash
./rubixgoplatform exp-peerdetails -did <did-address> -port <port-number>
```

**Description:** Adds peer details sourced from explorer data.

**Options:**
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/add-peer-details-from-explorer`

---

### Token Status & Monitoring

#### Get Pledged Token Details

Check pledged token information.

```bash
./rubixgoplatform get-pledged-token-details -port <port-number>
```

**Description:** Retrieves details of pledged tokens.

**Options:**
- `-port string`: Server/Host port (default: "20000").

**Related API:** `GET /api/get-pledgedtoken-details`

---

#### Check Pinned State

Check if tokens are in a pinned state.

```bash
./rubixgoplatform check-pinned-state -tokenstatehash <tokenstate-hash> -port <port-number>
```

**Description:** Verifies the pinned state of tokens and return pinned peers.

**Options:**
- `-tokenstatehash string`: Token State Hash to check pinned state.
- `-port string`: Server/Host port (default: "20000").

**Related API:** `GET /api/check-pinned-state`

---

#### Run Unpledge

Execute unpledge operations.

```bash
./rubixgoplatform run-unpledge -port <port-number>
```

**Description:** Performs unpledge operations for pledged tokens.

**Options:**
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/run-unpledge`

---

#### Unpledge POW Pledge Tokens

Unpledge Proof-of-Work pledged tokens.

```bash
./rubixgoplatform unpledge-pow-pledge-tokens -port <port-number>
```

**Description:** Unpledges tokens used in Proof-of-Work.

**Options:**
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/unpledge-pow-unpledge-tokens`

---

### Transaction Management

#### Get Transaction Details

Retrieve transaction details by various parameters.

```bash
./rubixgoplatform get-txn-details -did <did-address> -txnID <transaction-ID> -transComment <transaction-comment> -port <port-number>
```

**Description:** Fetches transaction details based on specified parameters.

**Options:**
- `-did string`: DID address.
- `-txnID string`: Transaction ID.
- `-transComment string`: Transaction comment.
- `-port string`: Server/Host port (default: "20000").

**Related APIs:**
- `GET /api/get-by-txnId`
- `GET /api/get-by-did`
- `GET /api/get-by-comment`
- `GET /api/get-by-node`

---

### Migration & Recovery

#### Migrate Node

Migrate an existing Java node to Rubix Go.

```bash
./rubixgoplatform migrate-node -fp
```

**Description:** Migrates a Java-based Rubix node to the Go platform.

**Options:**
- `-fp`: Force password entry.

**Related API:** `POST /api/migrate-node`

---

## API Reference

### External APIs

All external APIs are accessible via HTTP requests to your node (default port: 20000).

#### Node Management

| Endpoint | Method | Description | CLI Command |
|----------|--------|-------------|-------------|
| `/api/start` | POST | Start the node | `run` |
| `/api/shutdown` | POST | Shutdown the node | `shutdown` |
| `/api/node-status` | GET | Get node status | N/A |
| `/api/ping` | GET | Ping the node | `ping` |
| `/api/get-peer-id` | GET | Get node's peer ID | `get-peer-id` |

#### Bootstrap Management

| Endpoint | Method | Description | CLI Command |
|----------|--------|-------------|-------------|
| `/api/add-bootstrap` | POST | Add bootstrap peers | `add-bootstrap` |
| `/api/remove-bootstrap` | POST | Remove bootstrap peers | `remove-bootstrap` |
| `/api/remove-all-bootstrap` | POST | Remove all bootstrap peers | `remove-all-bootstrap` |
| `/api/get-all-bootstrap` | GET | Get all bootstrap peers | `get-all-bootstrap` |

#### DID Management

| Endpoint | Method | Description | CLI Command |
|----------|--------|-------------|-------------|
| `/api/createdid` | POST | Create a new DID | `create-did` |
| `/api/getalldid` | GET | Get all DIDs | `get-all-did` |
| `/api/register-did` | POST | Register DID on network | `register-did` |
| `/api/setup-did` | POST | Setup DID configuration | `setup-did` |
| `/api/getdidchallenge` | GET | Get DID challenge | N/A |
| `/api/logindid` | POST | Login with DID | N/A |
| `/api/request-did-for-pubkey` | POST | Request DID for public key | `create-did-from-pubkey` |

#### Token Management

| Endpoint | Method | Description | CLI Command |
|----------|--------|-------------|-------------|
| `/api/generate-test-token` | POST | Generate test RBT tokens | `generate-test-rbt` |
| `/api/generate-faucettest-token` | POST | Generate faucet test tokens | `generate-faucet-rbt` |
| `/api/initiate-rbt-transfer` | POST | Transfer RBT tokens | `transfer-rbt` |
| `/api/initiate-self-transfer` | POST | Self-transfer RBT tokens | `self-transfer-rbt` |
| `/api/get-account-info` | GET | Get account information | `get-account-info` |
| `/api/getalltokens` | GET | Get all tokens | N/A |
| `/api/validate-token-chain` | POST | Validate token chain | `validate-tokenchain` |
| `/api/validate-token` | POST | Validate specific token | `validatetoken` |
| `/api/lock-tokens` | POST | Lock tokens | `lock-tokens` |
| `/api/release-all-locked-tokens` | POST | Release all locked tokens | `release-all-locked-tokens` |
| `/api/initiate-pin-token` | POST | Pin tokens | `pin-token` |
| `/api/recover-token` | POST | Recover tokens | `recover-token` |
| `/api/faucet-token-check` | GET | Check faucet tokens | `faucet-token-check` |

#### Token Chain Operations

| Endpoint | Method | Description | CLI Command |
|----------|--------|-------------|-------------|
| `/api/dump-token-chain` | GET | Dump token chain data | `dump-tokenchain` |
| `/api/remove-token-chain-block` | POST | Remove token chain block | N/A |

#### FT Operations

| Endpoint | Method | Description | CLI Command |
|----------|--------|-------------|-------------|
| `/api/create-ft` | POST | Create fungible tokens | `create-ft` |
| `/api/initiate-ft-transfer` | POST | Transfer fungible tokens | `transfer-ft` |
| `/api/get-ft-info-by-did` | GET | Get FT info by DID | `get-ft-info-by-did` |
| `/api/dump-ft-token-chain` | GET | Dump FT token chain | `dump-ft` |
| `/api/get-ft-token-chain` | GET | Get FT token chain data | N/A |
| `/api/get-ft-txn-by-did` | GET | Get FT transactions by DID | `get-ft-txn-details` |

#### NFT Operations

| Endpoint | Method | Description | CLI Command |
|----------|--------|-------------|-------------|
| `/api/create-nft` | POST | Create NFT | `create-nft` |
| `/api/list-nfts` | GET | List all NFTs | `get-all-nft` |
| `/api/addnftsale` | POST | Add NFT for sale | N/A |
| `/api/deploy-nft` | POST | Deploy NFT contract | `deploy-nft` |
| `/api/execute-nft` | POST | Execute NFT transaction | `execute-nft` |
| `/api/dump-nft-token-chain` | GET | Dump NFT token chain | `dump-nft-tokenchain` |
| `/api/subscribe-nft` | POST | Subscribe to NFT | `subscribe-nft` |
| `/api/get-nft-token-chain-data` | GET | Get NFT token chain data | N/A |
| `/api/fetch-nft` | GET | Fetch NFT details | `fetch-nft` |
| `/api/get-nfts-by-did` | GET | Get NFTs by DID | `get-nfts-by-did` |

#### Smart Contract Operations

| Endpoint | Method | Description | CLI Command |
|----------|--------|-------------|-------------|
| `/api/deploy-smart-contract` | POST | Deploy smart contract | `deploy-smartcontract` |
| `/api/execute-smart-contract` | POST | Execute smart contract | `execute-smartcontract` |
| `/api/generate-smart-contract` | POST | Generate smart contract | `generate-sct` |
| `/api/fetch-smart-contract` | GET | Fetch smart contract | `fetch-sct` |
| `/api/publish-smart-contract` | POST | Publish smart contract | `publish-sct` |
| `/api/subscribe-smart-contract` | POST | Subscribe to smart contract | `subscribe-sct` |
| `/api/dump-smart-contract-token-chain` | GET | Dump smart contract token chain | `dump-smartcontract-tokenchain` |
| `/api/get-smart-contract-token-chain-data` | GET | Get smart contract token chain data | `get-smartcontract-data` |

#### Data Token Operations

| Endpoint | Method | Description | CLI Command |
|----------|--------|-------------|-------------|
| `/api/create-data-token` | POST | Create data token | `create-datatoken` |
| `/api/commit-data-token` | POST | Commit data token | `commit-datatoken` |
| `/api/check-data-token` | GET | Check data token | N/A |
| `/api/get-data-token` | GET | Get data token | N/A |

#### Quorum Management

| Endpoint | Method | Description | CLI Command |
|----------|--------|-------------|-------------|
| `/api/addquorum` | POST | Add quorum list | `add-quorum` |
| `/api/getallquorum` | GET | Get all quorum configs | `get-all-quorum` |
| `/api/removeallquorum` | POST | Remove all quorum configs | `remove-all-quorum` |
| `/api/setup-quorum` | POST | Setup quorum | `setup-quorum` |
| `/api/check-quorum-status` | GET | Check quorum status | `check-quorum-status` |

#### Explorer & Monitoring

| Endpoint | Method | Description | CLI Command |
|----------|--------|-------------|-------------|
| `/api/add-explorer` | POST | Add explorer URL | `add-explorer` |
| `/api/remove-explorer` | POST | Remove explorer URL | `remove-explorer` |
| `/api/get-all-explorer` | GET | Get all explorer URLs | `get-all-explorer` |
| `/api/get-pledgedtoken-details` | GET | Get pledged token details | `get-pledged-token-details` |
| `/api/check-pinned-state` | GET | Check pinned state | `check-pinned-state` |
| `/api/run-unpledge` | POST | Run unpledge operation | `run-unpledge` |
| `/api/unpledge-pow-unpledge-tokens` | POST | Unpledge POW tokens | `unpledge-pow-pledge-tokens` |

#### Transaction Queries

| Endpoint | Method | Description | CLI Command |
|----------|--------|-------------|-------------|
| `/api/get-by-txnId` | GET | Get transaction by ID | `get-txn-details` |
| `/api/get-by-did` | GET | Get transactions by DID | `get-txn-details` |
| `/api/get-by-comment` | GET | Get transactions by comment | `get-txn-details` |
| `/api/get-by-node` | GET | Get transactions by node | `get-txn-details` |

#### Service & Database

| Endpoint | Method | Description | CLI Command |
|----------|--------|-------------|-------------|
| `/api/setup-service` | POST | Setup service | `setup-service` |
| `/api/setup-db` | POST | Setup database | `setup-db` |

#### Peer Management

| Endpoint | Method | Description | CLI Command |
|----------|--------|-------------|-------------|
| `/api/add-peer-details` | POST | Add peer details | `add-peer-details` |
| `/api/add-peer-details-from-explorer` | POST | Add peer details from explorer | `exp-peerdetails` |

#### Migration & Recovery

| Endpoint | Method | Description | CLI Command |
|----------|--------|-------------|-------------|
| `/api/migrate-node` | POST | Migrate node | `migrate-node` |

#### User Management

| Endpoint | Method | Description | CLI Command |
|----------|--------|-------------|-------------|
| `/api/add-user-api-key` | POST | Add user API key | `add-user-apikey` |
| `/api/send-jwt-from-wallet` | POST | Send JWT from wallet | N/A |

#### Additional Operations

| Endpoint | Method | Description | CLI Command |
|----------|--------|-------------|-------------|
| `/api/signature-response` | POST | Handle signature response | N/A |
| `/api/register-callback-url` | POST | Register callback URL | N/A |
| `/api/get-token-status` | GET | Get token status | N/A |
| `/api/update-token-status` | POST | Update token status | N/A |

## Support and Community

For additional support:
- Check the official documentation
- Join the community forums
- Submit issues to the project repository
- Contact technical support team

## Version Compatibility

Check version compatibility:
```bash
./rubixgoplatform -v
```

Ensure all nodes in your network are running compatible versions for proper operation.

---

**Note:** This documentation covers the comprehensive set of commands and APIs available in Rubix Go Platform. Always use test networks for development and testing purposes. For production deployments, follow security best practices and proper configuration management.