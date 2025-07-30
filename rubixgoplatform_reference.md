# Rubix Go Platform

Rubix is a quantum‑immune, highly scalable layer‑1 blockchain protocol powered by Proof‑of‑Pledge consensus. It provides decentralized identity (DID), native tokens (RBT, FT, NFT), smart contracts, and secure peer-to-peer storage.

Explore more at:
- 🌐 [rubix.net](https://rubix.net)
- 📚 [learn.rubix.net](https://learn.rubix.net)
- 🧑‍💻 [Rubix GitHub](https://github.com/rubixchain)

---

## ⚙️ Usage Overview

Rubix Go Platform offers:
- **CLI Interface (`rubixgoplatform`)** for node operations, token creation, smart contract deployment
- **REST API Interface** for web and programmatic access via HTTP

---

## 🔌 API Reference (Sample Payloads & Responses)

> All endpoints default to `http://<your-node-ip>:20000`

| Endpoint                          | Method | Description                      |
|----------------------------------|--------|----------------------------------|
| `/api/createdid`                 | POST   | Create a new DID                 |
| `/api/initiate-rbt-transfer`     | POST   | Transfer RBT tokens              |
| `/api/create-ft`                 | POST   | Create fungible tokens           |
| `/api/transfer-ft`               | POST   | Transfer fungible tokens         |
| `/api/create-nft`                | POST   | Create an NFT                    |
| `/api/deploy-smart-contract`     | POST   | Deploy a smart contract          |
| `/api/execute-smart-contract`    | POST   | Execute smart contract function  |
| `/api/get-account-info`          | GET    | Fetch account info for a DID     |

### Sample: `/api/createdid`

**Request:**
```json
{
  "didType": 1,
  "didSecret": "MySecret123",
  "privPWD": "MyPrivatePassword"
}
```

**Response:**
```json
{
  "status": true,
  "did": "did:rubix:bafybeibcf3...",
  "message": "DID created successfully"
}
```

*(See [learn.rubix.net](https://learn.rubix.net/api/) for full details)*

---

## 🖥️ CLI Command Reference

Command syntax:
```bash
./rubixgoplatform <command> [flags]
```

---

### 🔧 Node Management

| Command     | Description                         |
|-------------|-------------------------------------|
| `run`       | Start a Rubix node instance         |
| `shutdown`  | Stop the Rubix node                 |
| `ping`      | Ping a peer node                    |

#### Example:
```bash
./rubixgoplatform run -p node1 -n 0 -s -testNet
```

---

### 🆔 DID Management

| Command          | Description                  |
|------------------|------------------------------|
| `create-did`     | Create a new DID             |
| `register-did`   | Register DID on network      |
| `setup-did`      | Setup the DID environment    |
| `get-all-did`    | List DIDs managed by node    |

#### Example:
```bash
./rubixgoplatform create-did -didType 1 -didSecret "RubixUser" -privPWD "MyPwd123" -fp
```

---

### 💰 Token Operations (RBT / FT)

#### RBT Commands

| Command               | Description             |
|-----------------------|-------------------------|
| `generate-test-rbt`   | Mint test RBT           |
| `transfer-rbt`        | Transfer RBT tokens     |
| `get-account-info`    | Fetch DID balance       |

#### FT Commands

| Command           | Description               |
|-------------------|---------------------------|
| `create-ft`       | Create fungible tokens    |
| `transfer-ft`     | Transfer FT to another DID|

#### Example:
```bash
./rubixgoplatform create-ft -did did:rubix:abc123 -ftName MyToken -ftCount 100 -rbtAmount 10
```

---

### 🖼️ NFT Operations

| Command                   | Description                |
|---------------------------|----------------------------|
| `create-nft`              | Create new NFT             |
| `deploy-nft`              | Deploy NFT to network      |
| `subscribe-nft`           | Subscribe to NFT           |
| `execute-nft`             | Trigger NFT function       |
| `fetch-nft`               | Get NFT metadata           |
| `dump-nft-tokenchain`     | Dump NFT chain history     |

#### Example:
```bash
./rubixgoplatform create-nft -did did:rubix:xyz456 -metadata ./meta.json -artifact ./image.png
```

---

### 🧠 Smart Contract Operations

| Command                     | Description                      |
|-----------------------------|----------------------------------|
| `generate-sct`              | Generate smart contract template |
| `deploy-smartcontract`      | Deploy smart contract            |
| `execute-smartcontract`     | Call a contract function         |
| `fetch-sct`                 | Fetch SCT by ID                  |
| `subscribe-sct`             | Subscribe to SCT events          |
| `dump-smartcontract-tokenchain` | Debug SCT token history     |

---

### 🔐 Quorum & Peer Management

| Command               | Description                  |
|-----------------------|------------------------------|
| `add-bootstrap`       | Add bootstrap node           |
| `remove-bootstrap`    | Remove bootstrap             |
| `add-quorum`          | Add node to quorum           |
| `setup-quorum`        | Configure local quorum       |
| `get-all-quorum`      | List quorum DIDs             |
| `add-peer-details`    | Add peer info manually       |
| `exp-peerdetails`     | Export peer list             |

---

## 📦 Data Tokens

| Command             | Description                    |
|---------------------|--------------------------------|
| `create-datatoken`  | Create a private data token    |
| `commit-datatoken`  | Commit token to chain          |

---

## 📚 References

- [Rubix Learn Portal](https://learn.rubix.net)
- [Rubix CLI Documentation](https://learn.rubix.net/nft-command/)
- [Rubix GitHub](https://github.com/rubixchain/rubixgoplatform)
- [API Reference](https://learn.rubix.net/api/)
- [FT CLI Reference](https://learn.rubix.net/ft-cli-command/)
- [Smart Contract CLI Reference](https://learn.rubix.net/sct-cli-command/)

---

## 🧑‍💻 Authors & Contributors

Maintained by the Rubix Core Team. Contributions welcome via GitHub PRs.

---