# Rubix Blockchain Documentation

## Table of Contents

1. Introduction to Rubix Blockchain
2. Setting Up a Rubix Node
3. Setting Up a Quorum
4. Creating a Decentralized Identity (DID)
5. Transferring RBT Tokens
6. Writing and Deploying Smart Contracts
7. Verifying a Transaction
8. Checking Node Status
9. Managing Fungible Tokens (FTs)
10. Pinging a Peer
11. Managing Bootstrap Peers
12. Getting Help and Version Information
13. Troubleshooting
14. Connect with Rubix Blockchain

---

## 1. Introduction to Rubix Blockchain

Rubix is a Layer-1 blockchain protocol focusing on decentralized identity (DID), security, and energy-efficient consensus mechanisms**. It employs the Proof of Pledge (PoP) consensus model, eliminating traditional mining and ensuring sustainability.

### Key Features:
- Decentralized Identity (DID): Provides self-sovereign identity for users.
- Proof of Pledge (PoP) Consensus: Secure and eco-friendly alternative to mining.
- Interoperability: Supports smart contracts using Rust and WASM.
- Privacy & Security: Uses cryptographic proofs, including Zero-Knowledge Proofs (ZKP).
- Scalability: High-throughput transactions with low fees.

---

## 2. Setting Up a Rubix Node

## Prerequisites Installation

### macOS

brew install git cmake make clang pkg-config openssl@1.1

### Linux

sudo apt update && sudo apt install -y git cmake make clang pkg-config libssl-dev


### Windows 

choco install git cmake make llvm openssl

### Download and Run the Rubix Node
### macOS & Linux

wget https://rubix-blockchain-binaries.s3.amazonaws.com/rubixgoplatform
chmod +x rubixgoplatform
./rubixgoplatform run -p node1 -n 0 -s -testNet -grpcPort 10500

### Windows (PowerShell)

Invoke-WebRequest -Uri "https://rubix-blockchain-binaries.s3.amazonaws.com/rubixgoplatform.exe" -OutFile "rubixgoplatform.exe"
./rubixgoplatform.exe run -p node1 -n 0 -s -testNet -grpcPort 10500

---

## 3. Setting Up a Quorum

./rubixgoplatform create-quorum -n 3  # Create a quorum with 3 nodes
./rubixgoplatform add-to-quorum -p node2
./rubixgoplatform add-to-quorum -p node3

---

## 4. Creating a Decentralized Identity (DID)

./rubixgoplatform create-did

This will generate a unique Decentralized Identity (DID) for the user.

---

## 5. Transferring RBT Tokens
### macOS & Linux

./rubixgoplatform transfer -to RECEIVER_DID -amount 10

### Windows

./rubixgoplatform.exe transfer -to RECEIVER_DID -amount 10

---

## 6. Writing and Deploying Smart Contracts

### Install Rust and WASM Toolchain
### macOS & Linux

curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
source $HOME/.cargo/env
rustup target add wasm32-unknown-unknown

### Windows 

iwr -useb https://sh.rustup.rs | iex
rustup target add wasm32-unknown-unknown

### Compile and Deploy Smart Contract

cargo build --target wasm32-unknown-unknown --release
./rubixgoplatform deploy-contract -f target/wasm32-unknown-unknown/release/contract.wasm

---

## 7. Verifying a Transaction

./rubixgoplatform verify-transaction -txid YOUR_TRANSACTION_ID

---

## 8. Checking Node Status

./rubixgoplatform status

---

## 9. Managing Fungible Tokens (FTs)

### Create Fungible Token (FT)

./rubixgoplatform create-ft -ftName TOKEN_NAME -ftCount 1000 -rbtAmount 10 -did YOUR_DID

### Transfer Fungible Token (FT)

./rubixgoplatform transfer-ft -senderAddr SENDER_ADDRESS -receiverAddr RECEIVER_ADDRESS -ftName TOKEN_NAME -ftCount 100 -transComment "Transfer comment" -transType 2

### Get FT Information by DID

./rubixgoplatform get-ft-info-by-did -did YOUR_DID

### Dump FT Token Chain

./rubixgoplatform dump-ft -token FT_TOKEN_ID

---

## 10. Pinging a Peer

./rubixgoplatform ping -peerID PEER_ID -port PORT_NUMBER

---

## 11. Managing Bootstrap Peers

### Add Bootstrap Peer

./rubixgoplatform addbootstrap -peers /ip4/103.60.213.76/tcp/4001/p2p/PEER_ID

### Remove Bootstrap Peer

./rubixgoplatform removebootstrap -peers /ip4/103.60.213.76/tcp/4001/p2p/PEER_ID

---

## 12. Getting Help and Version Information

### Display Help Information

./rubixgoplatform -h

### Display Version Information

./rubixgoplatform -v

---

## 13. Troubleshooting

### Permission Issues (macOS/Linux)

chmod +x rubixgoplatform

---

## 14. Connect with Rubix Blockchain
- Website: https://rubix.net
- GitHub: https://github.com/rubixchain
- X: https://twitter.com/RubixBlockchain
- Discord: https://discord.gg/rubixblockchain
- Email: support@rubix.net
