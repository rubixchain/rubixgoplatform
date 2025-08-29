<p align="center">
  <img src="Rubix_logo.png" alt="Rubix Logo" width="300"/>
</p>

<h1 align="center">RUBIX NETWORKING</h1>

<p align="center">
  <b>An L1 Non-monolithic Blockchain Protocol</b>
</p>

---

Welcome to the **Rubix Go Platform**! This README provides comprehensive documentation of the platform’s command-line interface (CLI) tools alongside RESTful API endpoints, enabling developers and operators to interact seamlessly with the Rubix blockchain node and its services.

---

## 🧰 Prerequisites

To run a **Rubix node**, use the latest [release binary](https://github.com/rubixchain/rubixgoplatform/releases) or build from source. The following tools are required for building from source:

### 📦 Required Tools (for Source Build)

| Tool                      | Version       | Purpose                                              |
|---------------------------|---------------|------------------------------------------------------|
| **Go**                    | 1.20+         | Build the `rubixgoplatform` binary via `make`        |
| **Make**                  | Latest        | Automates the build process                          |
| **IPFS Binary**           | v0.21.0       | Enables decentralized storage                        |
| **Git**                   | Latest        | Clones the Rubix repository                          |
| **Test Swarm Key**        | Latest        | (`testswarm.key`) Required to join the Rubix Testnet |
| **Swarm Key**             | Latest        | (`swarm.key`) Required to join the Rubix Mainnet     |

## ⚙️ Quick Setup (Build from Source)

To build and run a Rubix node from source:

### Clone the Repository

```bash
git clone https://github.com/rubixchain/rubixgoplatform.git
cd rubixgoplatform
```

### Build the Binary

```bash
# For macOS:
make compile-mac

# For Linux:
make compile-linux

# For Windows:
make compile-windows

# Remove existing binary:
make clean
```

- Find the binary file in the respective OS folder.
- Add the IPFS binary file to the respective OS folder.
- Move the `testswarm.key`/`swarm.key` from the root folder (`rubixgoplatform`) to the respective OS folder.

---

*Ensure all nodes are running the latest version for proper operation.*

---

## 🔧 CLI Commands

<!-- <details>
  <summary><strong>CLI Commands</strong></summary>

- [Basic Commands](#basic-commands)
- [Node Management](#node-management)
- [Bootstrap Management](#bootstrap-management)
- [Peer Management](#peer-management)
- [DID Management](#did-management)
- [Quorum Management](#quorum-management)
- [Token Management](#token-management)
- [Token Status & Monitoring](#token-status--monitoring)
- [Token Chain Operations](#token-chain-operations)
- [FT Operations](#ft-operations)
- [NFT Operations](#nft-operations)
- [Smart Contract Operations](#smart-contract-operations)
- [Explorer Management](#explorer-management)
- [Transaction Management](#transaction-management)
- [Migration & Recovery](#migration--recovery)

</details> -->

### Basic Commands

<details>
  <summary><strong>Version</strong></summary>

Display the version of the Rubix Go Platform executable.

```bash
./rubixgoplatform -v
```

**Description:** Retrieves the current version of the Rubix Go Platform.

**Options:**
- `-v`: Display the executable version.

---

</details>

<details>
  <summary><strong>Help</strong></summary>

Display help information for available CLI commands and options.

```bash
./rubixgoplatform -h
```

**Description:** Shows a list of available commands and their usage.

**Options:**
- `-h`: Show help information of commands.

</details>

---

### Node Management

<details>
  <summary><strong>Run Node</strong></summary>

Start a Rubix blockchain node.

```bash
./rubixgoplatform run -p <node-name> -n <node-number> -s -testNet -grpcPort <grpc-port-number>
```

**Description:** Initializes and runs the Rubix node, enabling core blockchain functionality.

**Options:**
- `-n uint`: Node number.
- `-p string`: Working directory path (default: "./").
- `-s`: Start the core.
- `-grpcPort string`: GRPC port number (default: "10500")
- `-testNet`: Run as test network.
- `-testNetKey string`: Test network key (default: "testswarm.key").

**Related API:** `POST /api/start`

---

</details>

<details>
  <summary><strong>Shutdown Node</strong></summary>

Shut down the Rubix node.

```bash
./rubixgoplatform shutdown -port <port-number>
```

**Description:** Gracefully terminates the Rubix node.

**Options:**
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/shutdown`

</details>

---

### Bootstrap Management

<details>
  <summary><strong>Add Bootstrap</strong></summary>

Add bootstrap peers to the Rubix node.

```bash
./rubixgoplatform add-bootstrap -peers <bootstrap-address> -port <port-number>
```

**Description:** Adds bootstrap peers to facilitate network discovery and connectivity.

**Options:**
- `-addr string`: Server/Host address (default: "localhost").
- `-peers string`: Bootstrap peers (comma-separated for multiple peers).
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/add-bootstrap`

---

</details>

<details>
  <summary><strong>Remove Bootstrap</strong></summary>

Remove specific bootstrap peers from the node.

```bash
./rubixgoplatform remove-bootstrap -peers <bootstrap-address> -port <port-number>
```

**Description:** Removes specified bootstrap peers from the node configuration.

**Options:**
- `-addr string`: Server/Host address (default: "localhost").
- `-peers string`: Bootstrap peers to remove (comma-separated).
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/remove-bootstrap`

---

</details>

<details>
  <summary><strong>Remove All Bootstrap</strong></summary>

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

</details>

<details>
  <summary><strong>Get All Bootstrap</strong></summary>

List all configured bootstrap peers.

```bash
./rubixgoplatform get-all-bootstrap -port <port-number>
```

**Description:** Retrieves a list of all bootstrap peers configured on the node.

**Options:**
- `-addr string`: Server/Host address (default: "localhost").
- `-port string`: Server/Host port (default: "20000").

**Related API:** `GET /api/get-all-bootstrap`

</details>

---

### Peer Management

<details>
  <summary><strong>Get Peer ID</strong></summary>

Retrieve the node's peer ID.

```bash
./rubixgoplatform get-peer-id -port <port-number>
```

**Description:** Fetches the unique peer ID of the running node.

**Options:**
- `-port string`: Server/Host port (default: "20000").

**Related API:** `GET /api/get-peer-id`

---

</details>

<details>
  <summary><strong>Ping Peer</strong></summary>

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

</details>

<details>
  <summary><strong>Add Peer Details</strong></summary>

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

</details>

<details>
  <summary><strong>Add Peer Details from Explorer</strong></summary>

Add peer details from explorer data.

```bash
./rubixgoplatform exp-peerdetails -did <did-address> -port <port-number>
```

**Description:** Adds peer details sourced from explorer data.

**Options:**
- `-did string`: DID address.
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/add-peer-details-from-explorer`

</details>

---

### DID Management

<details>
  <summary><strong>Create DID</strong></summary>

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

</details>

<details>
  <summary><strong>Create DID from Public Key</strong></summary>

Create a DID from an existing public key.

```bash
./rubixgoplatform create-did-from-pubkey -pubKeyFile <public-key-file> -port <port-number>
```

**Description:** Generates a DID using a provided public key.

**Options:**
- `-pubKeyFile string`: Public key file (default: "").
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/request-did-for-pubkey`

---

</details>

<details>
  <summary><strong>Get All DIDs</strong></summary>

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

</details>

<details>
  <summary><strong>Register DID</strong></summary>

Register a DID and PeerID mapping on the network.

```bash
./rubixgoplatform register-did -did <did-address> -port <port-number>
```

**Description:** Registers a DID with its associated PeerID on the Rubix network.

**Options:**
- `-did string`: DID address.
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/register-did`

</details>

---

### Quorum Management

<details>
  <summary><strong>Add Quorum</strong></summary>

Add a quorum list to the node.

```bash
./rubixgoplatform add-quorum -quorumList <quorumlist-file> -port <port-number>
```

**Description:** Configures a quorum list for the node.

**Options:**
- `-quorumList string`: Quorum list file name (default: "quorumlist.json").
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/addquorum`

---

</details>

<details>
  <summary><strong>Setup Quorum</strong></summary>

Set up quorum to enable participation in consensus.

```bash
./rubixgoplatform setup-quorum -did <did-address> -port <port-number>
```

**Description:** Initializes quorum for the given DID using the provided passwords, enabling participation in consensus.

**Options:**
- `-did string`: DID address.
- `-port string`: Server/Host port (default: "20000").
- `-quorumPWD string`: Quorum key password (default: "mypassword").
- `-privPWD string`: Private key password (default: "mypassword").
- `-fp`: Force password entry in terminal.

**Related API:** `POST /api/setup-quorum`

---

</details>

<details>
  <summary><strong>Get All Quorum</strong></summary>

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

</details>

<details>
  <summary><strong>Remove All Quorum</strong></summary>

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

</details>

<details>
  <summary><strong>Check Quorum Status</strong></summary>

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

</details>

---

### Token Management

<details>
  <summary><strong>Generate Test RBT</strong></summary>

Generate test RBT tokens on the node.

```bash
./rubixgoplatform generate-test-rbt -did <did-address> -numTokens <token-amount> -port <port-number>
```

**Description:** Creates test RBT tokens for testing in local network.

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

</details>

<!-- <details>
  <summary><strong>Generate Faucet Test RBT</strong></summary>

Generate faucet test RBT tokens.

```bash
./rubixgoplatform generate-faucet-rbt -did <did-address> -numTokens <token-amount> -port <port-number>
```

**Description:** Creates faucet test RBT tokens for testing in Rubix Testnet.

**Options:**
- `-did string`: DID address.
- `-numTokens int`: Number of tokens to generate (default: 1).
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/generate-faucettest-token`

---

</details> -->

<details>
  <summary><strong>Transfer RBT</strong></summary>

Transfer RBT tokens between addresses.

```bash
./rubixgoplatform transfer-rbt -senderAddr <sender-did> -receiverAddr <receiver-did> -rbtAmount <token-amount> -port <port-number>
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

</details>

<!-- <details>
  <summary><strong>Self Transfer RBT</strong></summary>

Perform self-transfer of RBT tokens.

```bash
./rubixgoplatform self-transfer-rbt -senderAddr <sender-did> -receiverAddr <receiver-did> -rbtAmount <token-amount> -port <port-number>
```

**Description:** Transfers RBT tokens to the same DID.

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

**Related API:** `POST /api/initiate-self-transfer`

</details> -->

<details>
  <summary><strong>Get Account Info</strong></summary>

Retrieve account information for a DID.

```bash
./rubixgoplatform get-account-info -did <did-address> -port <port-number>
```

**Description:** Fetches account details for a specified DID.

**Options:**
- `-did string`: DID address.
- `-port string`: Server/Host port (default: "20000").

**Related API:** `GET /api/get-account-info`

</details>

<!-- <details>
  <summary><strong>Release All Locked Tokens</strong></summary>

Release all locked tokens.

```bash
./rubixgoplatform release-all-locked-tokens -port <port-number>
```

**Description:** Unlocks all previously locked tokens.

**Options:**
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/release-all-locked-tokens`

</details> -->

<details>
  <summary><strong>Pin Token</strong></summary>

Pin RBT tokens for pinning service.

```bash
./rubixgoplatform pin-token -pinningAddress <pinning-node-did-address> -senderAddr <sender-address> -rbtAmount <token-amount> -port <port-number>
```

**Description:** Pinning of RBT tokens as a service.

**Options:**
- `-pinningAddress string`: Pinning DID address.
- `-senderAddr string`: Sender address.
- `-rbtAmount float`: RBT token amount.
- `-transComment string`: Transfer comment (default: "Test transaction").
- `-transType int`: Transaction type (default: 2).
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/initiate-pin-token`

---

</details>

<details>
  <summary><strong>Recover Tokens</strong></summary>

Recover RBT tokens after pinning for pinning service.

```bash
./rubixgoplatform recover-token -pinningAddress <pinning-node-did-address> -senderAddr <sender-address> -rbtAmount <token-amount> -port <port-number>
```

**Description:** Recovers RBT tokens from pinning service.

**Options:**
- `-pinningAddress string`: Pinning DID address.
- `-senderAddr string`: Sender address.
- `-rbtAmount float`: RBT token amount.
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/recover-token`

---

</details>

<details>
  <summary><strong>Validate Token</strong></summary>

Validate a specific token.

```bash
./rubixgoplatform validate-token -token <token-ID> -port <port-number>
```

**Description:** Verifies the existance of a specific token.

**Options:**
- `-token string`: Token ID.
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/validate-token`

---

</details>

<!-- <details>
  <summary><strong>Faucet Token Check</strong></summary>

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

</details> -->

---

### Token Status & Monitoring

<details>
  <summary><strong>Get Pledged Token Details</strong></summary>

Check pledged token information.

```bash
./rubixgoplatform get-pledged-token-details -port <port-number>
```

**Description:** Retrieves details of pledged tokens.

**Options:**
- `-port string`: Server/Host port (default: "20000").

**Related API:** `GET /api/get-pledgedtoken-details`

---

</details>

<details>
  <summary><strong>Check Pinned State</strong></summary>

Check if tokens are in a pinned state.

```bash
./rubixgoplatform check-pinned-state -tokenstatehash <tokenstate-hash> -port <port-number>
```

**Description:** Verifies the pinned state of tokens and returns pinned peers.

**Options:**
- `-tokenstatehash string`: Token state hash to check pinned state.
- `-port string`: Server/Host port (default: "20000").

**Related API:** `GET /api/check-pinned-state`

---

</details>

<details>
  <summary><strong>Run Unpledge</strong></summary>

Execute unpledge operations.

```bash
./rubixgoplatform run-unpledge -port <port-number>
```

**Description:** Performs unpledge operations for pledged tokens.

**Options:**
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/run-unpledge`

---

</details>

<!-- <details>
  <summary><strong>Unpledge POW Pledge Tokens</strong></summary>

Unpledge Proof-of-Work pledged tokens.

```bash
./rubixgoplatform unpledge-pow-pledge-tokens -port <port-number>
```

**Description:** Unpledges tokens used in Proof-of-Work.

**Options:**
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/unpledge-pow-unpledge-tokens`

</details> -->

---

### Token Chain Operations

<details>
  <summary><strong>Dump Token Chain</strong></summary>

Get RBT token chain data.

```bash
./rubixgoplatform dump-tokenchain -token <token-id> -port <port-number>
```

**Description:** Exports data from an RBT token chain to a JSON file.

**Options:**
- `-token string`: Token ID.
- `-port string`: Server/Host port (default: "20000").

**Related API:** `GET /api/dump-token-chain`

**Note:** The token chain data will be saved as a JSON file named `dump.json` in the current directory.

---

</details>

<details>
  <summary><strong>Decode Token Chain</strong></summary>

Decode dumped token chain data.

```bash
./rubixgoplatform decode-tokenchain
```

**Description:** Decodes previously dumped token chain (`dump.json`) data for better readability.

---

</details>

<details>
  <summary><strong>Validate Token Chain</strong></summary>

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
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/validate-token-chain`

</details>

---

### FT Operations

<details>
  <summary><strong>Create FT</strong></summary>

Create fungible tokens.

```bash
./rubixgoplatform create-ft -did <did-address> -ftName <ft-token-name> -ftCount <ft-token-amount> -rbtAmount <token-amount> -port <port-number>
```

**Description:** Generates fungible tokens (FTs) on the node.

**Options:**
- `-did string`: DID address.
- `-ftName string`: Name of the FT.
- `-ftCount int`: Number of FTs to create.
- `-rbtAmount int`: RBT amount for FT creation.
- `-port string`: Server/Host port (default: "20000").
- `-ftStartIndex int`: FT number start index (default: 0).

**Note:** Use the `-ftStartIndex` flag when creating additional FTs with the same DID address and FT name to ensure numbering continues from the correct index.

**Related API:** `POST /api/create-ft`

---

</details>

<details>
  <summary><strong>Transfer FT</strong></summary>

Transfer fungible tokens.

```bash
./rubixgoplatform transfer-ft -ftName <ft-token-name> -ftCount <ft-token-amount> -senderAddr <sender-did> -receiverAddr <receiver-did> -port <port-number>
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

**Related API:** `POST /api/initiate-ft-transfer`

**Note:** Use the `-creatorDID` flag when transferring FTs with duplicate names created by different DIDs to accurately identify and transfer the correct FT batch.

---

</details>

<details>
  <summary><strong>Get FT Info by DID</strong></summary>

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

</details>

<details>
  <summary><strong>Dump FT Token Chain</strong></summary>

Get FT token chain data.

```bash
./rubixgoplatform dump-ft -token <ft-token-id> -port <port-number>
```

**Description:** Exports data from an FT token chain to a JSON file.

**Options:**
- `-token string`: FT token ID.
- `-port string`: Server/Host port (default: "20000").

**Related API:** `GET /api/dump-ft-token-chain`

**Note:** The token chain data will be saved as a JSON file named `dump.json` in the current directory.

---

</details>

<details>
  <summary><strong>Get FT Transaction Details</strong></summary>

Retrieve FT transaction details by DID.

```bash
./rubixgoplatform get-ft-txn-details -did <did-address> -port <port-number>
```

**Description:** Fetches transaction details for FTs associated with a DID.

**Options:**
- `-did string`: DID address.
- `-port string`: Server/Host port (default: "20000").

**Related API:** `GET /api/get-ft-txn-by-did`

</details>

---

### NFT Operations

<details>
  <summary><strong>Create NFT</strong></summary>

Create Non-Fungible Tokens (NFTs).

```bash
./rubixgoplatform create-nft -did <did-address> -metadata <metadata-json-file> -artifact <artifact-file> -port <port-number>
```

**Description:** Generates new NFTs on the node.

**Options:**
- `-did string`: DID address.
- `-port string`: Server/Host port (default: "20000").
- `-metadata string`: Path of JSON file containing information about the NFT (default: "").
- `-artifact string`: Path of file meant to be an NFT (default: "").

**Related API:** `POST /api/create-nft`

---

</details>

<details>
  <summary><strong>Deploy NFT</strong></summary>

Deploy an NFT smart contract.

```bash
./rubixgoplatform deploy-nft -deployerAddr <deployer-did-address> -nft <nft-ID> -nftValue <nft-token-amount> -port <port-number>
```

**Description:** Deploys an NFT smart contract to the network.

**Options:**
- `-nft string`: NFT ID.
- `-deployerAddr string`: DID address of the deployer of the NFT.
- `-quorumType int`: Quorum type (default: 2).
- `-nftValue float`: Value of the NFT (default: 0.0).
- `-nftData string`: Arbitrary data associated with NFT (default: "").
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/deploy-nft`

---

</details>

<details>
  <summary><strong>Execute NFT</strong></summary>

Execute NFT transactions.

```bash
./rubixgoplatform execute-nft -nft <nft-ID> -executorAddr <executor-did-address> -receiver <receiver-address> -rbtAmount <token-amount> -nftData <data> -port <port-number>
```

**Description:** Performs NFT transactions.

**Options:**
- `-nft string`: NFT ID (default: "").
- `-executorAddr string`: DID address of the executor of the NFT (default: "").
- `-receiver string`: DID that receives the ownership of NFT (default: "").
- `-transType int`: Quorum type (default: 2).
- `-transComment string`: Transaction comment (default: "").
- `-rbtAmount float`: Sale value of NFT (default: 0.0).
- `-nftData string`: Arbitrary data associated with NFT (default: "").
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/execute-nft`

---

</details>

<details>
  <summary><strong>Get All NFT</strong></summary>

List all NFTs.

```bash
./rubixgoplatform get-all-nft -did <did-address> -port <port-number>
```

**Description:** Retrieves a list of all NFTs on the node.

**Options:**
- `-did string`: DID address.
- `-port string`: Server/Host port (default: "20000").

**Related API:** `GET /api/list-nfts`

---

</details>

<details>
  <summary><strong>Subscribe NFT</strong></summary>

Subscribe to NFT events.

```bash
./rubixgoplatform subscribe-nft -nft <nft-ID> -port <port-number>
```

**Description:** Subscribes to NFT token chain updates.

**Options:**
- `-nft string`: NFT ID (default: "").
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/subscribe-nft`

---

</details>

<details>
  <summary><strong>Fetch NFT</strong></summary>

Fetch NFT details.

```bash
./rubixgoplatform fetch-nft -nft <nft-ID> -port <port-number>
```

**Description:** Retrieves detailed information of an NFT from the network to the node.

**Options:**
- `-nft string`: NFT ID (default: "").
- `-port string`: Server/Host port (default: "20000").

**Related API:** `GET /api/fetch-nft`

---

</details>

<details>
  <summary><strong>Get NFTs by DID</strong></summary>

Retrieve NFTs owned by a specific DID.

```bash
./rubixgoplatform get-nfts-by-did -did <did-address> -port <port-number>
```

**Description:** Lists all NFTs associated with a given DID.

**Options:**
- `-did string`: DID address.
- `-port string`: Server/Host port (default: "20000").

**Related API:** `GET /api/get-nfts-by-did`

---

</details>

<details>
  <summary><strong>Dump NFT Token Chain</strong></summary>

Export NFT token chain data.

```bash
./rubixgoplatform dump-nft-tokenchain -nft <nft-ID> -port <port-number>
```

**Description:** Exports data from an NFT token chain to a JSON file.

**Options:**
- `-nft string`: NFT ID (default: "").
- `-port string`: Server/Host port (default: "20000").

**Related API:** `GET /api/dump-nft-token-chain`

</details>

---

### Smart Contract Operations

<details>
  <summary><strong>Generate Smart Contract</strong></summary>

Generate smart contract code.

```bash
./rubixgoplatform generate-sct -did <did-address> -binCode <binary-wasm-file> -rawCode <raw-contract-file> -schemaFile <state-change-json-file> -port <port-number>
```

**Description:** Creates smart contract token for deployment.

**Options:**
- `-did string`: DID address.
- `-port string`: Server/Host port (default: "20000").
- `-binCode string`: Path of WASM file compiled from raw contract (default: "").
- `-rawCode string`: Path of raw smart contract code file (default: "").
- `-schemaFile string`: Path of JSON file used to track state changes (default: "").

**Related API:** `POST /api/generate-smart-contract`

---

</details>

<details>
  <summary><strong>Deploy Smart Contract</strong></summary>

Deploy a smart contract to the network.

```bash
./rubixgoplatform deploy-smartcontract -sct <smartcontract-ID> -deployerAddr <deployer-did-address> -rbtAmount <token-amount> -transType <quorum-type> -port <port-number>
```

**Description:** Deploys a smart contract to the Rubix network.

**Options:**
- `-sct string`: Smart contract ID (default: "").
- `-deployerAddr string`: DID address of the deployer of the smart contract.
- `-rbtAmount float`: Value of the smart contract (default: 0.0).
- `-transType int`: Quorum type (default: 2).
- `-transComment string`: Transaction comment (default: "").
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/deploy-smart-contract`

---

</details>

<details>
  <summary><strong>Execute Smart Contract</strong></summary>

Execute smart contract functions.

```bash
./rubixgoplatform execute-smartcontract -sct <smartcontract-ID> -executorAddr <executor-did-address> -transType <quorum-type> -sctData <data> -port <port-number>
```

**Description:** Runs specified functions within a smart contract.

**Options:**
- `-sct string`: Smart contract ID (default: "").
- `-executorAddr string`: DID address of the executor of the smart contract.
- `-transType int`: Quorum type (default: 2).
- `-transComment string`: Transaction comment (default: "").
- `-sctData string`: Arbitrary data associated with smart contract (default: "").
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/execute-smart-contract`

---

</details>

<details>
  <summary><strong>Fetch Smart Contract</strong></summary>

Fetch smart contract details.

```bash
./rubixgoplatform fetch-sct -sct <smartcontract-ID> -port <port-number>
```

**Description:** Retrieves details of a smart contract.

**Options:**
- `-sct string`: Smart contract ID (default: "").
- `-port string`: Server/Host port (default: "20000").

**Related API:** `GET /api/fetch-smart-contract`

---

</details>

<details>
  <summary><strong>Publish Smart Contract</strong></summary>

Publish a smart contract to the network.

```bash
./rubixgoplatform publish-sct -sct <smartcontract-ID> -did <publisher-did-address> -pubType 1 -sctBlockHash <block-hash> -port <port-number>
```

**Description:** Publishes a smart contract for network-wide access.

**Options:**
- `-sct string`: Smart contract ID.
- `-did string`: Publisher DID address.
- `-pubType int`: Smart contract event publishing type (Deploy: 1, Execute: 2; default: 0).
- `-sctBlockHash string`: Smart contract block.
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/publish-smart-contract`

---

</details>

<details>
  <summary><strong>Subscribe Smart Contract</strong></summary>

Subscribe to smart contract events.

```bash
./rubixgoplatform subscribe-sct -sct <smartcontract-ID> -port <port-number>
```

**Description:** Subscribes to events emitted by a smart contract.

**Options:**
- `-sct string`: Smart contract ID.
- `-port string`: Server/Host port (default: "20000").

**Related API:** `POST /api/subscribe-smart-contract`

---

</details>

<details>
  <summary><strong>Dump Smart Contract Token Chain</strong></summary>

Export smart contract token chain data.

```bash
./rubixgoplatform dump-smartcontract-tokenchain -sct <smartcontract-ID> -port <port-number>
```

**Description:** Saves data from a smart contract token chain to a JSON file.

**Options:**
- `-sct string`: Smart contract ID.
- `-port string`: Server/Host port (default: "20000").

**Related API:** `GET /api/dump-smart-contract-token-chain`

---

</details>

<details>
  <summary><strong>Get Smart Contract Data</strong></summary>

Retrieve smart contract token chain data.

```bash
./rubixgoplatform get-smartcontract-data -sct <smartcontract-ID> -port <port-number>
```

**Description:** Fetches data from a smart contract token chain.

**Options:**
- `-sct string`: Smart contract ID.
- `-port string`: Server/Host port (default: "20000").

**Related API:** `GET /api/get-smart-contract-token-chain-data`

</details>

---

### Explorer Management

<details>
  <summary><strong>Add Explorer</strong></summary>

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

</details>

<details>
  <summary><strong>Remove Explorer</strong></summary>

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

</details>

<details>
  <summary><strong>Get All Explorer</strong></summary>

List all configured explorer URLs.

```bash
./rubixgoplatform get-all-explorer -port 20000
```

**Description:** Retrieves a list of all configured explorer URLs.

**Options:**
- `-port string`: Server/Host port (default: "20000").

**Related API:** `GET /api/get-all-explorer`

---

</details>

<details>
  <summary><strong>Add User API Key</strong></summary>

Add an API key for user authentication.

```bash
./rubixgoplatform add-user-apikey -did <did-address> -apiKey <api-key> -port <port-number>
```

**Description:** Adds an API key for authenticating users.

**Options:**
- `-did string`: DID address.
- `-port string`: Server/Host port (default: "20000").
- `-apiKey string`: API key corresponding to DID.

**Related API:** `POST /api/add-user-api-key`

</details>

---

### Transaction Management

<details>
  <summary><strong>Get Transaction Details</strong></summary>

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

</details>

---

### Migration & Recovery

<details>
  <summary><strong>Migrate Node</strong></summary>

Migrate an existing Java node to Rubix Go.

```bash
./rubixgoplatform migrate-node -fp
```

**Description:** Migrates a Java-based Rubix node to the Go platform.

**Options:**
- `-fp`: Force password entry.

**Related API:** `POST /api/migrate-node`

</details>

---

## 🌐 API Endpoints

All below APIs are accessible via HTTP requests to your node (default port: 20000).

<details>
  <summary><strong>Node Management</strong></summary>

| Endpoint              | Method | Description          |
|-----------------------|--------|----------------------|
| `/api/start`          | POST   | Start the node       |
| `/api/shutdown`       | POST   | Shutdown the node    |
| `/api/node-status`    | GET    | Get node status      |

</details>

---

<details>
  <summary><strong>Bootstrap Management</strong></summary>

| Endpoint                    | Method | Description                  |
|-----------------------------|--------|------------------------------|
| `/api/add-bootstrap`        | POST   | Add bootstrap peers          |
| `/api/remove-bootstrap`     | POST   | Remove bootstrap peers       |
| `/api/remove-all-bootstrap` | POST   | Remove all bootstrap peers   |
| `/api/get-all-bootstrap`    | GET    | Get all bootstrap peers      |

</details>

---

<details>
  <summary><strong>Peer Management</strong></summary>

| Endpoint                             | Method | Description                         |
|--------------------------------------|--------|-------------------------------------|
| `/api/ping`                          | GET    | Ping the node                       |
| `/api/get-peer-id`                   | GET    | Get node's peer ID                  |
| `/api/add-peer-details`              | POST   | Add peer details                    |
| `/api/add-peer-details-from-explorer`| POST   | Add peer details from explorer      |

</details>

---

<details>
  <summary><strong>DID Management</strong></summary>

| Endpoint                         | Method | Description                                 |
|----------------------------------|--------|---------------------------------------------|
| `/api/create-did`                | POST   | Create a new DID                            |
| `/api/get-all-did`               | GET    | Get all DIDs                                |
| `/api/register-did`              | POST   | Register DID on network                     |
| `/api/setup-did`                 | POST   | Setup DID configuration                     |
| `/api/login-did`                 | POST   | Get DID access                              |
| `/api/request-did-for-pubkey`    | POST   | Create DID from given pubKey                |
| `/api/send-jwt-from-wallet`      | POST   | Authenticate RBT transfer JWT from wallet   |

</details>

---

<details>
  <summary><strong>Quorum Management</strong></summary>

| Endpoint                             | Method | Description                         |
|--------------------------------------|--------|-------------------------------------|
| `/api/add-quorum`                    | POST   | Add quorum list                     |
| `/api/get-all-quorum`                | GET    | Get all quorum configs              |
| `/api/remove-all-quorum`             | POST   | Remove all quorum configs           |
| `/api/setup-quorum`                  | POST   | Setup quorum                        |
| `/api/check-quorum-status`           | GET    | Check quorum status                 |

</details>

---

<details>
  <summary><strong>Token Management</strong></summary>

| Endpoint                              | Method | Description                           |
|---------------------------------------|--------|---------------------------------------|
| `/api/generate-test-token`            | POST   | Generate test RBT tokens              |
| `/api/generate-faucet-test-token`     | POST   | Generate faucet test tokens           |
| `/api/initiate-rbt-transfer`          | POST   | Transfer RBT tokens                   |
| `/api/initiate-self-transfer`         | POST   | Self-transfer RBT tokens (within DID) |
| `/api/get-account-info`               | GET    | Get account information               |
| `/api/get-all-tokens`                 | GET    | Get all tokens                        |
| `/api/validate-token`                 | POST   | Validate specific token               |
| `/api/lock-tokens`                    | POST   | Lock tokens                           |
| `/api/release-all-locked-tokens`      | POST   | Release all locked tokens             |
| `/api/initiate-pin-token`             | POST   | Pin tokens for pinning service        |
| `/api/recover-token`                  | POST   | Recover tokens after pinning service  |
| `/api/faucet-token-check`             | GET    | Check faucet tokens                   |

</details>

---

<details>
  <summary><strong>Token Status & Monitoring</strong></summary>

| Endpoint                              | Method | Description                           |
|---------------------------------------|--------|---------------------------------------|
| `/api/get-token-status`               | GET    | Get token status                      |
| `/api/update-token-status`            | POST   | Update token status                   |
| `/api/get-pledged-token-details`      | GET    | Get pledged token details             |
| `/api/check-pinned-state`             | GET    | Check pinned state of token state     |
| `/api/run-unpledge`                   | POST   | Unpledge pledged tokens               |
| `/api/unpledge-pow-unpledge-tokens`   | POST   | Unpledge POW tokens                   |

</details>

---

<details>
  <summary><strong>Token Chain Operations</strong></summary>

| Endpoint                        | Method | Description                    |
|---------------------------------|--------|--------------------------------|
| `/api/dump-token-chain`         | GET    | Dump token chain data to JSON  |
| `/api/validate-token-chain`     | POST   | Validate token chain           |

</details>

---

<details>
  <summary><strong>FT Operations</strong></summary>

| Endpoint                        | Method | Description                         |
|---------------------------------|--------|-------------------------------------|
| `/api/create-ft`                | POST   | Create fungible tokens              |
| `/api/initiate-ft-transfer`     | POST   | Transfer fungible tokens            |
| `/api/get-ft-info-by-did`       | GET    | Get FT info by DID                  |
| `/api/dump-ft-token-chain`      | GET    | Dump FT token chain                 |
| `/api/get-ft-token-chain`       | GET    | View FT token chain data            |
| `/api/get-ft-txn-by-did`        | GET    | Get FT transactions by DID          |

</details>

---

<details>
  <summary><strong>NFT Operations</strong></summary>

| Endpoint                          | Method | Description                        |
|-----------------------------------|--------|------------------------------------|
| `/api/create-nft`                 | POST   | Create NFT                         |
| `/api/list-nfts`                  | GET    | List all NFTs                      |
| `/api/deploy-nft`                 | POST   | Deploy NFT contract                |
| `/api/execute-nft`                | POST   | Execute NFT transaction            |
| `/api/dump-nft-token-chain`       | GET    | Dump NFT token chain               |
| `/api/subscribe-nft`              | POST   | Subscribe to NFT                   |
| `/api/get-nft-token-chain-data`   | GET    | Get NFT token chain data           |
| `/api/fetch-nft`                  | GET    | Fetch NFT details                  |
| `/api/get-nfts-by-did`            | GET    | Get NFTs by DID                    |
| `/api/add-nft-sale`               | POST   | Add NFT for sale                   |

</details>

---

<details>
  <summary><strong>Smart Contract Operations</strong></summary>

| Endpoint                                   | Method | Description                            |
|--------------------------------------------|--------|----------------------------------------|
| `/api/deploy-smart-contract`               | POST   | Deploy smart contract                  |
| `/api/execute-smart-contract`              | POST   | Execute smart contract                 |
| `/api/generate-smart-contract`             | POST   | Generate smart contract                |
| `/api/fetch-smart-contract`                | GET    | Fetch smart contract                   |
| `/api/publish-smart-contract`              | POST   | Publish smart contract                 |
| `/api/subscribe-smart-contract`            | POST   | Subscribe to smart contract            |
| `/api/dump-smart-contract-token-chain`     | GET    | Dump smart contract token chain        |
| `/api/get-smart-contract-token-chain-data` | GET    | Get smart contract token chain data    |
| `/api/register-callback-url`               | POST   | Register callback URL                  |

</details>

---

<details>
  <summary><strong>Explorer Management</strong></summary>

| Endpoint                              | Method | Description                          |
|---------------------------------------|--------|--------------------------------------|
| `/api/add-explorer`                   | POST   | Add explorer URL                     |
| `/api/remove-explorer`                | POST   | Remove explorer URL                  |
| `/api/get-all-explorer`               | GET    | Get all explorer URLs                |
| `/api/add-user-api-key`               | POST   | Add user API key                     |

</details>

---

<details>
  <summary><strong>Transaction Queries</strong></summary>

| Endpoint              | Method | Description                 |
|-----------------------|--------|-----------------------------|
| `/api/get-by-txnId`   | GET    | Get transaction by ID       |
| `/api/get-by-did`     | GET    | Get transactions by DID     |
| `/api/get-by-comment` | GET    | Get transactions by comment |
| `/api/get-by-node`    | GET    | Get transactions by node    |

</details>

---

<details>
  <summary><strong>System & Utility</strong></summary>

| Endpoint                     | Method | Description                    |
|------------------------------|--------|--------------------------------|
| `/api/setup-service`         | POST   | Setup service                  |
| `/api/setup-db`              | POST   | Setup database                 |
| `/api/signature-response`    | POST   | Handle signature response      |
| `/api/migrate-node`          | POST   | Migrate node                   |

</details>

---

## 🤝 Support and Community

For additional support:
- Checkout 
    - [Rubix website](https://rubix.net/)
    - [Rubix Learn website](https://learn.rubix.net/).
- Join the community forums.
- Submit issues to the [Issues page](https://github.com/rubixchain/rubixgoplatform/issues).
- Contact the technical support team.

---

**Note:** This documentation covers the comprehensive set of commands and APIs available in the Rubix Go Platform. Always use test networks for development and testing purposes. For production deployments, follow security best practices and proper configuration management.
