<p align="center">
  <img src="Rubix_logo.png" alt="Your Organization Logo" width="150"/>
</p>

<h1 align="center">RUBIX</h1>

<p align="center">
  <b>Empowering decentralized systems</b>
</p>

---
# Rubix Go Platform

Welcome to the RubixGoPlatform !!!
This README provides comprehensive documentation of the platform’s command-line interface (CLI) tools alongside RESTful API endpoints, enabling developers and operators to easily interact with the Rubix blockchain node and its services.

## Table of Contents

- [CLI Commands](#cli-commands)
- [API Reference](#api-reference)

## CLI Commands

### Basic Commands

#### Version & Help
```bash
# Get executable version
./rubixgoplatform -v

# Get help
./rubixgoplatform -h
```

### Node Management

#### Run Node
Start the Rubix blockchain node.

```bash
./rubixgoplatform run [OPTIONS]
```

**Options:**
- `-n uint` - Node number
- `-p string` - Working directory path (default "./")
- `-s` - Start the core
- `-testNet` - Run as test net
- `-testNetKey string` - Test net key (default "testswarm.key")

**Example:**
```bash
./rubixgoplatform run -p node1 -n 0 -s -testNet
```

**Related API:** `POST /api/start`

#### Shutdown Node
Shutdown the Rubix node.

```bash
./rubixgoplatform shutdown
```

**Related API:** `POST /api/shutdown`

#### Ping Peer
Test connectivity with network peers.

```bash
./rubixgoplatform ping [OPTIONS]
```

**Options:**
- `-addr string` - Server/Host Address (default "localhost")
- `-peerID string` - Peer ID
- `-port string` - Server/Host port (default "20000")

**Example:**
```bash
./rubixgoplatform ping -peerID 12D3KooWKr8dEQiLXuKacxDCZiHePVEMpgjxk19C3QozuUVQcQHA -port 20000
```

**Related API:** `GET /api/ping`

#### Get Peer ID
Get the node's peer ID.

```bash
./rubixgoplatform get-peer-id [OPTIONS]
```

**Related API:** `GET /api/get-peer-id`

### Bootstrap Management

#### Add Bootstrap
Add bootstrap peers to the network.

```bash
./rubixgoplatform add-bootstrap [OPTIONS]
```

**Options:**
- `-addr string` - Server/Host Address (default "localhost")
- `-peers string` - Bootstrap peers (comma-separated for multiple)
- `-port string` - Server/Host port (default "20000")

**Example:**
```bash
./rubixgoplatform add-bootstrap -peers /ip4/103.60.213.76/tcp/4001/p2p/QmR1VH6SsEN1wf4EmstxXtNMvR35KEetbBetiGWWKWavJ6
```

**Related API:** `POST /api/add-bootstrap`

#### Remove Bootstrap
Remove specific bootstrap peers.

```bash
./rubixgoplatform remove-bootstrap [OPTIONS]
```

**Options:**
- `-addr string` - Server/Host Address (default "localhost")
- `-peers string` - Bootstrap peers to remove (comma-separated)
- `-port string` - Server/Host port (default "20000")

**Related API:** `POST /api/remove-bootstrap`

#### Remove All Bootstrap
Remove all bootstrap peers from the node.

```bash
./rubixgoplatform remove-all-bootstrap [OPTIONS]
```

**Related API:** `POST /api/remove-all-bootstrap`

#### Get All Bootstrap
List all bootstrap peers.

```bash
./rubixgoplatform get-all-bootstrap [OPTIONS]
```

**Related API:** `GET /api/get-all-bootstrap`

### DID Management

#### Create DID
Create a new Decentralized Identifier.

```bash
./rubixgoplatform create-did [OPTIONS]
```

**Options:**
- `-port string` - Server/Host port (default "20000")
- `-didType int` - DID type: 0=Basic, 1=Standard, 2=Wallet, 3=Child, 4=Light (default 0)
- `-didSecret string` - DID secret (default "My DID Secret")
- `-privPWD string` - Private key password (default "mypassword")
- `-quorumPWD string` - Quorum key password (default "mypassword")
- `-imgFile string` - Image file for DID creation - must be 256x256 PNG (default "image.png")
- `-didImgFile string` - DID image file name (default "did.png")
- `-privImgFile string` - Private share image file name (default "pvtShare.png")
- `-pubImgFile string` - Public share image file name (default "pubShare.png")
- `-privKeyFile string` - Private key file name (default "pvtKey.pem")
- `-pubKeyFile string` - Public key file name (default "pubKey.pem")
- `-mnemonicKeyFile string` - Mnemonic key file (default "mnemonic.txt")
- `-ChildPath int` - BIP Child Path (default 0)
- `-fp` - Force password entry in terminal

**Example:**
```bash
./rubixgoplatform create-did -didType 1 -didSecret "My Secure DID" -fp
```

**Related API:** `POST /api/createdid`

#### Create DID from Public Key
Create a DID from an existing public key.

```bash
./rubixgoplatform create-did-from-pubkey [OPTIONS]
```

**Related API:** `POST /api/request-did-for-pubkey`

#### Get All DIDs
List all DIDs on the node.

```bash
./rubixgoplatform get-all-did [OPTIONS]
```

**Related API:** `GET /api/getalldid`

#### Register DID
Register DID & PeerID mapping on the network.

```bash
./rubixgoplatform register-did [OPTIONS]
```

**Options:**
- `-did string` - DID address

**Related API:** `POST /api/register-did`

#### Setup DID
Setup DID configuration.

```bash
./rubixgoplatform setup-did [OPTIONS]
```

**Related API:** `POST /api/setup-did`

### Quorum Management

#### Add Quorum
Add quorum list to the node.

```bash
./rubixgoplatform add-quorum [OPTIONS]
```

**Options:**
- `-quorumList string` - Quorum list file name (default "quorumlist.json")

**Related API:** `POST /api/addquorum`

#### Setup Quorum
Setup quorum with private key password.

```bash
./rubixgoplatform setup-quorum [OPTIONS]
```

**Options:**
- `-quorumPWD string` - Quorum key password (default "mypassword")
- `-privPWD string` - Private key password (default "mypassword")
- `-fp` - Force password entry in terminal

**Related API:** `POST /api/setup-quorum`

#### Get All Quorum
List all quorum configurations.

```bash
./rubixgoplatform get-all-quorum
```

**Related API:** `GET /api/getallquorum`

#### Remove All Quorum
Remove all quorum configurations.

```bash
./rubixgoplatform remove-all-quorum
```

**Related API:** `POST /api/removeallquorum`

#### Check Quorum Status
Check the status of quorum configuration.

```bash
./rubixgoplatform check-quorum-status [OPTIONS]
```

**Related API:** `GET /api/check-quorum-status`

### Token Management

#### Generate Test RBT
Generate test RBT tokens on the node.

```bash
./rubixgoplatform generate-test-rbt [OPTIONS]
```

**Options:**
- `-did string` - DID address
- `-numTokens int` - Number of tokens to generate (default 1)
- `-fp` - Force password entry
- `-privPWD string` - Private key password (default "mypassword")
- `-privImgFile string` - Private share image file (default "pvtShare.png")
- `-privKeyFile string` - Private key file (default "pvtKey.pem")

**Related API:** `POST /api/generate-test-token`

#### Generate Faucet Test RBT
Generate faucet test RBT tokens.

```bash
./rubixgoplatform generate-faucet-rbt [OPTIONS]
```

**Related API:** `POST /api/generate-faucettest-token`

#### Transfer RBT
Transfer RBT tokens between addresses.

```bash
./rubixgoplatform transfer-rbt [OPTIONS]
```

**Options:**
- `-senderAddr string` - Sender address
- `-receiverAddr string` - Receiver address
- `-rbtAmount float` - RBT amount to transfer
- `-transComment string` - Transfer comment (default "Test transaction")
- `-transType int` - Transaction type (default 2)
- `-fp` - Force password entry
- `-privPWD string` - Private key password
- `-privImgFile string` - Private share image file
- `-privKeyFile string` - Private key file

**Related API:** `POST /api/initiate-rbt-transfer`

#### Self Transfer RBT
Perform self-transfer of RBT tokens.

```bash
./rubixgoplatform self-transfer-rbt [OPTIONS]
```

**Related API:** `POST /api/initiate-self-transfer`

#### Get Account Info
Get account information for a DID.

```bash
./rubixgoplatform get-account-info [OPTIONS]
```

**Options:**
- `-did string` - DID address

**Related API:** `GET /api/get-account-info`

#### Lock Tokens
Lock tokens for specific operations.

```bash
./rubixgoplatform lock-tokens [OPTIONS]
```

**Related API:** `POST /api/lock-tokens`

#### Release All Locked Tokens
Release all locked tokens.

```bash
./rubixgoplatform release-all-locked-tokens [OPTIONS]
```

**Related API:** `POST /api/release-all-locked-tokens`

#### Pin Token
Pin tokens for security purposes.

```bash
./rubixgoplatform pin-token [OPTIONS]
```

**Related API:** `POST /api/initiate-pin-token`

#### Recover Tokens
Recover RBT tokens.

```bash
./rubixgoplatform recover-token [OPTIONS]
```

**Related API:** `POST /api/recover-token`

#### Validate Token
Validate a specific token.

```bash
./rubixgoplatform validatetoken [OPTIONS]
```

**Related API:** `POST /api/validate-token`

#### Faucet Token Check
Check faucet token status.

```bash
./rubixgoplatform faucet-token-check [OPTIONS]
```

**Related API:** `GET /api/faucet-token-check`

### Token Chain Operations

#### Dump Token Chain
Export token chain data.

```bash
./rubixgoplatform dump-tokenchain [OPTIONS]
```

**Options:**
- `-token string` - Token address

**Related API:** `GET /api/dump-token-chain`

#### Decode Token Chain
Decode dumped token chain data.

```bash
./rubixgoplatform decode-tokenchain [OPTIONS]
```

#### Validate Token Chain
Validate RBT and smart contract token chains.

```bash
./rubixgoplatform validate-tokenchain [OPTIONS]
```

**Options:**
- `-did string` - DID address
- `-sctValidation bool` - Enable smart contract validation (default false)
- `-token string` - Token ID
- `-allmyTokens bool` - Validate all tokens (default false)
- `-blockCount int` - Number of blocks to validate (default 0 = all)

**Related API:** `POST /api/validate-token-chain`

#### Faucet Token Chain Validate
Validate faucet token chain.

```bash
./rubixgoplatform faucet-tokenchain-validate [OPTIONS]
```

#### Get Token Block
Get specific token block information.

```bash
./rubixgoplatform get-tokenblock [OPTIONS]
```

### Fungible Token (FT) Operations

#### Create FT
Create fungible tokens.

```bash
./rubixgoplatform create-ft [OPTIONS]
```

**Options:**
- `-did string` - DID address
- `-ftName string` - Name of the FT
- `-ftCount int` - Number of FTs to create
- `-rbtAmount int` - RBT amount for FT creation

**Related API:** `POST /api/create-ft`

#### Transfer FT
Transfer fungible tokens.

```bash
./rubixgoplatform transfer-ft [OPTIONS]
```

**Options:**
- `-ftName string` - FT name to transfer
- `-ftCount int` - Number of FTs to transfer
- `-senderAddr string` - Sender address
- `-receiverAddr string` - Receiver address
- `-transType int` - Transaction type (default 2)
- `-fp` - Force password authentication
- `-creatorDID string` - FT Creator DID (for multiple FTs with same name)

**Related API:** `POST /api/initiate-ft-transfer`

#### Get FT Info by DID
Get information about all FTs for a DID.

```bash
./rubixgoplatform get-ft-info-by-did [OPTIONS]
```

**Options:**
- `-did string` - DID address

**Related API:** `GET /api/get-ft-info-by-did`

#### Dump FT Token Chain
Export FT token chain data.

```bash
./rubixgoplatform dump-ft [OPTIONS]
```

**Options:**
- `-token string` - FT token ID

**Related API:** `GET /api/dump-ft-token-chain`

#### Get FT Transaction Details
Get FT transaction details by DID.

```bash
./rubixgoplatform get-ft-txn-details [OPTIONS]
```

**Related API:** `GET /api/get-ft-txn-by-did`

### NFT Operations

#### Create NFT
Create Non-Fungible Tokens.

```bash
./rubixgoplatform create-nft [OPTIONS]
```

**Related API:** `POST /api/create-nft`

#### Get All NFT
List all NFTs.

```bash
./rubixgoplatform get-all-nft [OPTIONS]
```

**Related API:** `GET /api/list-nfts`

#### Deploy NFT
Deploy NFT smart contract.

```bash
./rubixgoplatform deploy-nft [OPTIONS]
```

**Related API:** `POST /api/deploy-nft`

#### Execute NFT
Execute NFT transactions.

```bash
./rubixgoplatform execute-nft [OPTIONS]
```

**Related API:** `POST /api/execute-nft`

#### Subscribe NFT
Subscribe to NFT events.

```bash
./rubixgoplatform subscribe-nft [OPTIONS]
```

**Related API:** `POST /api/subscribe-nft`

#### Fetch NFT
Fetch NFT details.

```bash
./rubixgoplatform fetch-nft [OPTIONS]
```

**Related API:** `GET /api/fetch-nft`

#### Get NFTs by DID
Get NFTs owned by a specific DID.

```bash
./rubixgoplatform get-nfts-by-did [OPTIONS]
```

**Related API:** `GET /api/get-nfts-by-did`

#### Dump NFT Token Chain
Export NFT token chain data.

```bash
./rubixgoplatform dump-nft-tokenchain [OPTIONS]
```

**Related API:** `GET /api/dump-nft-token-chain`

### Smart Contract Operations

#### Generate Smart Contract
Generate smart contract code.

```bash
./rubixgoplatform generate-sct [OPTIONS]
```

**Related API:** `POST /api/generate-smart-contract`

#### Deploy Smart Contract
Deploy smart contract to the network.

```bash
./rubixgoplatform deploy-smartcontract [OPTIONS]
```

**Related API:** `POST /api/deploy-smart-contract`

#### Execute Smart Contract
Execute smart contract functions.

```bash
./rubixgoplatform execute-smartcontract [OPTIONS]
```

**Related API:** `POST /api/execute-smart-contract`

#### Fetch Smart Contract
Fetch smart contract details.

```bash
./rubixgoplatform fetch-sct [OPTIONS]
```

**Related API:** `GET /api/fetch-smart-contract`

#### Publish Smart Contract
Publish smart contract to the network.

```bash
./rubixgoplatform publish-sct [OPTIONS]
```

**Related API:** `POST /api/publish-smart-contract`

#### Subscribe Smart Contract
Subscribe to smart contract events.

```bash
./rubixgoplatform subscribe-sct [OPTIONS]
```

**Related API:** `POST /api/subscribe-smart-contract`

#### Dump Smart Contract Token Chain
Export smart contract token chain data.

```bash
./rubixgoplatform dump-smartcontract-tokenchain [OPTIONS]
```

**Related API:** `GET /api/dump-smart-contract-token-chain`

#### Get Smart Contract Data
Get smart contract token chain data.

```bash
./rubixgoplatform get-smartcontract-data [OPTIONS]
```

**Related API:** `GET /api/get-smart-contract-token-chain-data`

### Data Token Operations

#### Create Data Token
Create data tokens for data storage.

```bash
./rubixgoplatform create-datatoken [OPTIONS]
```

**Related API:** `POST /api/create-data-token`

#### Commit Data Token
Commit data token to the network.

```bash
./rubixgoplatform commit-datatoken [OPTIONS]
```

**Related API:** `POST /api/commit-data-token`

### Service Management

#### Setup Service
Configure services on the node.

```bash
./rubixgoplatform setup-service [OPTIONS]
```

**Options:**
- `-srvName string` - Service name (default "explorer_service")
- `-dbAddress string` - Database address (default "localhost")
- `-dbName string` - Database name (default "ExplorerDB")
- `-dbPassword string` - Database password (default "password")
- `-dbPort string` - Database port (default "1433")
- `-dbType string` - Database type: SQLServer, PostgreSQL, MySQL, Sqlite3 (default "SQLServer")
- `-dbUsername string` - Database username (default "sa")

**Related API:** `POST /api/setup-service`

#### Setup Database
Setup database configuration.

```bash
./rubixgoplatform setup-db [OPTIONS]
```

**Related API:** `POST /api/setup-db`

#### Update Configuration
Update node configuration.

```bash
./rubixgoplatform update-config [OPTIONS]
```

### Explorer Management

#### Add Explorer
Add explorer URLs for transaction data.

```bash
./rubixgoplatform add-explorer [OPTIONS]
```

**Options:**
- `-links string` - URLs (comma-separated for multiple)

**Related API:** `POST /api/add-explorer`

#### Remove Explorer
Remove explorer URLs.

```bash
./rubixgoplatform remove-explorer [OPTIONS]
```

**Options:**
- `-links string` - URLs to remove (comma-separated)

**Related API:** `POST /api/remove-explorer`

#### Get All Explorer
List all configured explorer URLs.

```bash
./rubixgoplatform get-all-explorer
```

**Related API:** `GET /api/get-all-explorer`

### Peer Management

#### Add Peer Details
Manually add peer details.

```bash
./rubixgoplatform add-peer-details [OPTIONS]
```

**Options:**
- `-peerID string` - Peer ID
- `-did string` - DID address
- `-didType int` - DID type (0-4)

**Related API:** `POST /api/add-peer-details`

#### Add Peer Details from Explorer
Add peer details from explorer data.

```bash
./rubixgoplatform exp-peerdetails [OPTIONS]
```

**Related API:** `POST /api/add-peer-details-from-explorer`

### Token Status & Monitoring

#### Get Pledged Token Details
Check pledged token information.

```bash
./rubixgoplatform get-pledged-token-details
```

**Related API:** `GET /api/get-pledgedtoken-details`

#### Check Pinned State
Check if tokens are in pinned state.

```bash
./rubixgoplatform check-pinned-state [OPTIONS]
```

**Related API:** `GET /api/check-pinned-state`

#### Run Unpledge
Execute unpledge operations.

```bash
./rubixgoplatform run-unpledge [OPTIONS]
```

**Related API:** `POST /api/run-unpledge`

#### Unpledge POW Pledge Tokens
Unpledge Proof-of-Work pledged tokens.

```bash
./rubixgoplatform unpledge-pow-pledge-tokens [OPTIONS]
```

**Related API:** `POST /api/unpledge-pow-unpledge-tokens`

### Transaction Management

#### Get Transaction Details
Get transaction details by various parameters.

```bash
./rubixgoplatform get-txn-details [OPTIONS]
```

**Related APIs:** 
- `GET /api/get-by-txnId`
- `GET /api/get-by-did`
- `GET /api/get-by-comment`
- `GET /api/get-by-node`

### Migration & Recovery

#### Migrate Node
Migrate existing Java node to RubixGo.

```bash
./rubixgoplatform migrate-node [OPTIONS]
```

**Options:**
- `-fp` - Force password entry

**Related API:** `POST /api/migrate-node`

### User Management

#### Add User API Key
Add API key for user authentication.

```bash
./rubixgoplatform add-user-apikey [OPTIONS]
```

**Related API:** `POST /api/add-user-api-key`

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

#### Fungible Token (FT) Operations

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

## Examples

### Basic Node Setup
```bash
# 1. Start the node
./rubixgoplatform run -p node1 -n 0 -s -testNet

# 2. Create a DID
./rubixgoplatform create-did -didType 1 -didSecret "My DID Secret"

# 3. Generate test tokens
./rubixgoplatform generate-test-rbt -did "your-did-address" -numTokens 10

# 4. Check account info
./rubixgoplatform get-account-info -did "your-did-address"
```

### Working with Bootstrap Peers
```bash
# Add bootstrap peers
./rubixgoplatform add-bootstrap -peers /ip4/103.60.213.76/tcp/4001/p2p/QmR1VH6SsEN1wf4EmstxXtNMvR35KEetbBetiGWWKWavJ6

# List all bootstrap peers
./rubixgoplatform get-all-bootstrap

# Remove specific bootstrap peer
./rubixgoplatform remove-bootstrap -peers /ip4/103.60.213.76/tcp/4001/p2p/QmR1VH6SsEN1wf4EmstxXtNMvR35KEetbBetiGWWKWavJ6

# Remove all bootstrap peers
./rubixgoplatform remove-all-bootstrap
```

### DID Operations
```bash
# Create a standard DID with custom settings
./rubixgoplatform create-did -didType 1 -didSecret "MySecureDID" -privPWD "securepass" -fp

# Create DID from existing public key
./rubixgoplatform create-did-from-pubkey -pubkey "your-public-key"

# Register DID on network
./rubixgoplatform register-did -did "your-did-address"

# Get all DIDs
./rubixgoplatform get-all-did
```

### Token Management
```bash
# Generate test RBT tokens
./rubixgoplatform generate-test-rbt -did "your-did" -numTokens 5

# Transfer RBT tokens
./rubixgoplatform transfer-rbt -senderAddr "sender-did" -receiverAddr "receiver-did" -rbtAmount 2.5

# Self-transfer tokens
./rubixgoplatform self-transfer-rbt -did "your-did" -amount 1.0

# Validate token chain
./rubixgoplatform validate-tokenchain -did "your-did" -allmyTokens true
```

### Fungible Token (FT) Operations
```bash
# Create fungible tokens
./rubixgoplatform create-ft -did "your-did" -ftName "MyToken" -ftCount 100 -rbtAmount 10

# Transfer FT tokens
./rubixgoplatform transfer-ft -ftName "MyToken" -ftCount 5 -senderAddr "sender-did" -receiverAddr "receiver-did"

# Get FT information
./rubixgoplatform get-ft-info-by-did -did "your-did"

# Dump FT token chain
./rubixgoplatform dump-ft -token "ft-token-id"
```

### NFT Operations
```bash
# Create NFT
./rubixgoplatform create-nft -did "your-did" -nftData "nft-metadata"

# Deploy NFT contract
./rubixgoplatform deploy-nft -contract "nft-contract-code"

# Get NFTs by DID
./rubixgoplatform get-nfts-by-did -did "your-did"

# Execute NFT transaction
./rubixgoplatform execute-nft -nftId "nft-id" -operation "transfer"
```

### Smart Contract Operations
```bash
# Generate smart contract
./rubixgoplatform generate-sct -contractType "basic" -parameters "param1,param2"

# Deploy smart contract
./rubixgoplatform deploy-smartcontract -contract "contract-code" -did "your-did"

# Execute smart contract
./rubixgoplatform execute-smartcontract -contractId "contract-id" -function "functionName" -args "arg1,arg2"

# Subscribe to smart contract events
./rubixgoplatform subscribe-sct -contractId "contract-id"
```

### Quorum Management
```bash
# Add quorum configuration
./rubixgoplatform add-quorum -quorumList "quorum.json"

# Setup quorum with passwords
./rubixgoplatform setup-quorum -quorumPWD "quorumpass" -privPWD "privatepass"

# Check quorum status
./rubixgoplatform check-quorum-status

# Remove all quorum configurations
./rubixgoplatform remove-all-quorum
```

### API Usage Examples

#### Check Node Status
```bash
curl -X GET http://localhost:20000/api/node-status
```

#### Create DID via API
```bash
curl -X POST http://localhost:20000/api/createdid \
  -H "Content-Type: application/json" \
  -d '{
    "didType": 1,
    "didSecret": "My DID Secret",
    "privPWD": "mypassword",
    "quorumPWD": "mypassword"
  }'
```

#### Generate Test Tokens via API
```bash
curl -X POST http://localhost:20000/api/generate-test-token \
  -H "Content-Type: application/json" \
  -d '{
    "did": "your-did-address",
    "numTokens": 5
  }'
```

#### Transfer RBT via API
```bash
curl -X POST http://localhost:20000/api/initiate-rbt-transfer \
  -H "Content-Type: application/json" \
  -d '{
    "senderAddr": "sender-did-address",
    "receiverAddr": "receiver-did-address",
    "rbtAmount": 5.0,
    "transComment": "Test transfer"
  }'
```

#### Create Fungible Token via API
```bash
curl -X POST http://localhost:20000/api/create-ft \
  -H "Content-Type: application/json" \
  -d '{
    "did": "your-did-address",
    "ftName": "MyToken",
    "ftCount": 100,
    "rbtAmount": 10
  }'
```

#### Get Account Information via API
```bash
curl -X GET "http://localhost:20000/api/get-account-info?did=your-did-address"
```

#### Add Bootstrap Peer via API
```bash
curl -X POST http://localhost:20000/api/add-bootstrap \
  -H "Content-Type: application/json" \
  -d '{
    "peers": "/ip4/103.60.213.76/tcp/4001/p2p/QmR1VH6SsEN1wf4EmstxXtNMvR35KEetbBetiGWWKWavJ6"
  }'
```

## Troubleshooting

### Common Issues

1. **Node won't start**
   - Check if port 20000 is available: `netstat -tulpn | grep 20000`
   - Verify working directory permissions: `ls -la`
   - Ensure testswarm.key exists for testnet mode
   - Check logs: `tail -f ./logs/rubix.log`

2. **DID creation fails**
   - Verify image file is exactly 256x256 PNG format
   - Check file permissions in working directory
   - Ensure passwords meet minimum requirements
   - Verify sufficient disk space

3. **Token transfer fails**
   - Verify sender has sufficient balance: `./rubixgoplatform get-account-info -did "sender-did"`
   - Check if quorum is properly setup: `./rubixgoplatform check-quorum-status`
   - Ensure both sender and receiver DIDs are registered
   - Verify network connectivity between peers

4. **API calls timeout or fail**
   - Check if node is running: `curl http://localhost:20000/api/ping`
   - Verify correct port and address configuration
   - Check network connectivity and firewall settings
   - Review API request format and required parameters

5. **Bootstrap peer connection issues**
   - Verify peer addresses are correctly formatted
   - Check network connectivity to bootstrap peers
   - Ensure bootstrap peers are online and accessible
   - Try with different bootstrap peers

6. **Quorum setup problems**
   - Verify quorum list file format is correct JSON
   - Check if quorum peers are accessible
   - Ensure passwords are correctly provided
   - Verify sufficient number of quorum members

7. **Smart contract deployment fails**
   - Check contract code syntax and format
   - Verify sufficient RBT balance for deployment
   - Ensure DID has necessary permissions
   - Review contract size limits

8. **Token validation errors**
   - Check token chain integrity
   - Verify all required signatures are present
   - Ensure proper token state transitions
   - Check for missing or corrupted token blocks

### Performance Optimization

1. **Node Performance**
   - Increase system resources (RAM, CPU)
   - Optimize database configuration
   - Use SSD storage for better I/O performance
   - Monitor and clean up log files regularly

2. **Network Performance**
   - Use reliable and fast internet connection
   - Configure appropriate number of bootstrap peers
   - Optimize peer discovery settings
   - Monitor network latency to peers

3. **Transaction Processing**
   - Batch multiple operations when possible
   - Use appropriate transaction types
   - Monitor transaction queue size
   - Optimize quorum configuration

### Log Files and Debugging

- **Node logs**: `./logs/rubix.log`
- **Error logs**: `./logs/error.log`
- **Debug logs**: `./logs/debug.log`
- **Transaction logs**: `./logs/transaction.log`

Enable debug mode for detailed logging:
```bash
./rubixgoplatform run -p node1 -n 0 -s -testNet -debug
```

### Configuration Files

- **Node configuration**: `./config/node.json`
- **Quorum configuration**: `./config/quorumlist.json`
- **Network configuration**: `./config/network.json`
- **Database configuration**: `./config/database.json`

### System Requirements

**Minimum Requirements:**
- OS: Linux (Ubuntu 18.04+), macOS, Windows 10+
- RAM: 4GB
- Storage: 20GB available space
- Network: Stable internet connection

**Recommended Requirements:**
- OS: Linux (Ubuntu 20.04+)
- RAM: 8GB+
- Storage: 100GB+ SSD
- Network: High-speed internet connection
- CPU: Multi-core processor

### Port Configuration

Default ports used by Rubix Go Platform:
- **HTTP API**: 20000
- **P2P Communication**: 4001
- **Bootstrap**: Various (configured per peer)

Ensure these ports are open in your firewall configuration.

### Database Setup

For production deployments:
1. Configure external database (PostgreSQL, MySQL, or SQL Server)
2. Set up proper database permissions
3. Configure connection pooling
4. Enable database backups
5. Monitor database performance

### Security Considerations

1. **Password Management**
   - Use strong, unique passwords for all accounts
   - Store passwords securely
   - Consider using hardware security modules

2. **Network Security**
   - Use secure connections (HTTPS/TLS)
   - Configure firewall rules appropriately
   - Monitor network traffic for anomalies

3. **File Permissions**
   - Restrict access to configuration files
   - Secure private key files
   - Regular security audits

4. **API Security**
   - Implement proper authentication
   - Use API keys for external access
   - Rate limiting for API endpoints

### Support and Community

For additional support:
- Check the official documentation
- Join the community forums
- Submit issues to the project repository
- Contact technical support team

### Version Compatibility

Check version compatibility:
```bash
./rubixgoplatform -v
```

Ensure all nodes in your network are running compatible versions for proper operation.

---

**Note:** This documentation covers the comprehensive set of commands and APIs available in Rubix Go Platform. Always use test networks for development and testing purposes. For production deployments, follow security best practices and proper configuration management.