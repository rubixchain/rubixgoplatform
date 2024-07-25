# Rubix CLI

The `command` package builds a structured and extensible Command-Line Interface for Rubix Blockchain Platform.

## Commands

Rubix CLI has the following top-level commands:

- [boostrap](#bootstrap-command)
- [chain-dump](#chain-dump-command)
- [config](#config-command)
- [did](#did-command)
- [explorer](#explorer-command)
- [node](#node-command)
  - [node peer](#node-peer)
  - [node pledge-tokens](#node-pledge-tokens)
- [quorum](#quorum-command)
- [run](#run-command)
- [tx](#tx-commands)
  - [tx rbt](#tx-rbt)
  - [tx smart-contract](#tx-smart-contract)
  - [tx nft](#tx-nft)
  - [tx data-token](#tx-data-token)
- [upgrade](#upgrade-command)
- [pin-service](#pin-service-command)
- [version](#version-command)

`addr` and `port` are Global Glags, which can be used in any command. Their default values are `localhost` and `20000` respectively.

Run `rubixgoplatform -h` to know more

### `bootstrap` Command

It consists of subcommands that are associated with managing boostrap nodes:

- `add`: Add IPFS bootstrap peers
- `list`: List all bootstrap peers from the configuration
- `remove`: Remove bootstrap peer(s) from the configuration
- `remove-all`: Removes all bootstrap peers from the configuration

To know more about the flags, run `rubixgoplatform bootstrap [command] --help`

### `chain-dump` Command

It consists of subcommands that are associated with Token chain and SmartContract chain dumps. 

- `smart-contract`: Get the dump of Smart Contract chain
- `token`: Get the dump of Token chain

To know more about the flags, run `rubixgoplatform chain-dump [command] --help`

### `config` Command

It consists of subcommands that are associated with setting up DB and Services. 

- `setup-db`: Setup Database
- `setup-service`: Setup Service

To know more about the flags, run `rubixgoplatform config [command] --help`

### `did` Command

It consists of subcommands that are associated with managing DIDs 

- `balance`: Get the account balance information of a DID
- `create`: Create a DID
- `list`: Fetch every DID present in the node
- `register`: Register DID

To know more about the flags, run `rubixgoplatform did [command] --help`

### `explorer` Command

It consists of subcommands that are associated with explorer. 

- `add`: Add Explorer URLs
- `list`: List all Explorer URLs
- `remove`: Remove Explorer URLs

To know more about the flags, run `rubixgoplatform explorer [command] --help`

### `node` Command

It consists of subcommands that are associated with Rubix node. 

- `lock-rbt-tokens`: Lock RBT tokens
- `migrate`: Migrate Node
- `peer`: Peer related subcommands
- `release-rbt-tokens`: Release all locked RBT tokens
- `shutdown`: shut down the node
- `pledge-tokens`: Pledge tokens related subcommands
- `token-state-pinned-info`: Check if a Token state is pinned

To know more about the flags, run `rubixgoplatform node [command] --help`

<a name="node-peer"></a>**`node peer` Commands**

It consists of subcommands associated with peer info of a node

- `add`: Add Peer details
- `local-id`: Get the local IPFS peer id
- `ping`: pings a peer
- `quorum-status`: check the status of quorum

To know more about the flags, run `rubixgoplatform node peer [command] --help`

<a name="node-pledge-tokens"></a>**`node pledge-tokens` Commands**

It consists of subcommands associated with pledge tokens of a node

- `list-token-states`: List all pledge token states of a node
- `unpledge`: Unpledge all pledged tokens

To know more about the flags, run `rubixgoplatform node pledge-tokens [command] --help`

### `quorum` Command

It consists of subcommands that are associated with Quorums. 

- `add`: Add addresses present with quorumlist.json in the node
- `list`: List all Quorums
- `remove-all`: Remove all Quorums
- `setup`: Setup up DID as a Quorum

To know more about the flags, run `rubixgoplatform quorum [command] --help`

### `run` Command

This command is used to run a Rubix node. To know more about the flags, run `rubixgoplatform run [command] --help`

### `tx` Commands

It consists of subcommands that are associated with transactions related to RBT tokens, Smart Contracts and NFT.

<a name="tx-rbt"></a>**`tx rbt` Commands**

It consists of subcommands associated with RBT tokens

- `generate-test-tokens`: Generate Test RBT tokens
- `transfer`: Transfer RBT tokens
- `self-transfer`: Self transfer RBT tokens

To know more about the flags, run `rubixgoplatform tx rbt [command] --help`

<a name="tx-smart-contract"></a>**`tx smart-contract` Commands**

It consists of subcommands associated with Smart Contracts

- `deploy`: Deploy a Smart Contract
- `execute`: Execute a Smart Contract
- `fetch`: Fetch a Smart Contract Token
- `generate`: Generate a Smart Contract Token
- `publish`: Publish a Smart Contract Token
- `subscribe`: Subscribe to a Smart Contract

To know more about the flags, run `rubixgoplatform tx smart-contract [command] --help`

<a name="tx-nft"></a>**`tx nft` Commands**

It consists of subcommands associated with NFTs

- `create`: Create an NFT
- `list`: List NFTs by DID

To know more about the flags, run `rubixgoplatform tx nft [command] --help`

<a name="tx-data-token"></a> **`tx data-token` Commands**

It consists of subcommands associated with Data tokens

- `commit`: Commit a Data Token
- `create`: Create a Data Token

To know more about the flags, run `rubixgoplatform tx data-token [command] --help`

### `pin-service` Command

It consists of subcommands associated with Token pinning and recovery

- `pin`: Pins a token on a pinning service provider node
- `recover`: Recovers the pinned token from the pinning service provider node

To know more about the flags, run `rubixgoplatform pin-service [command] --help`

### `upgrade` Command

It consists of subcommands associated with operations related to node version migrations

- `unpledge-pow-tokens`: Unpledge any pledge tokens which were pledged as part of PoW based pledging

To know more about the flags, run `rubixgoplatform upgrade [command] --help`

### `version` Command

This command will output the Rubix binary version.
