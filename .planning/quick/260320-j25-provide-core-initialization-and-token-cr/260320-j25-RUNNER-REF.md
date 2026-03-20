# Runner Setup Reference

**Branch:** release-v1
**Purpose:** Copy-paste-ready reference for setting up a test runner that initializes Core and creates/reads tokens without the full server/API layer.

---

## Quick Start Sequence

1. `config.ParseConfigFromPath(dir)` — reads `config.toml` from `dir`, returns `types.UserConfig`
2. `config.CreateRubixConfigFromUserConfig(userConfig, dir)` — derives `types.RubixConfig` (ports, DB config, network dirs)
3. `core.NewCore(&cfg, log, "localnet", false, false, false, "")` — constructs Core; internally:
   - `storage.NewRubixDB(ctx, &cfg.DBConfig, dbOpts)` — opens pgxpool, pings DB
   - `wallet.NewWallet(ctx, rubixDB, log)` — runs `InitSchema` (all CREATE TABLE IF NOT EXISTS), seeds `did_algo`, `token_role`, `token_type`
4. Core is now usable for direct wallet calls. `SetupCore()` (IPFS, p2p) has NOT been called.
5. Insert a DID row into `dids` before creating tokens (FK requirement).
6. Call `w.PersistGenesisTokenRecord(txRecord, token, entry)` to create genesis tokens directly.

---

## 1. Core Initialization

### Config Loading

```go
// core/config/config.go:56-69 — ParseConfigFromPath reads config.toml
func ParseConfigFromPath(configPath string) (types.UserConfig, error) {
    configFilePath := path.Join(configPath, "config.toml")
    configDataBytes, err := os.ReadFile(configFilePath)
    // ...
    var rubixConfig types.UserConfig
    if err := toml.Unmarshal(configDataBytes, &rubixConfig); err != nil { ... }
    return rubixConfig, nil
}
```

```go
// core/config/config.go:85-133 — CreateRubixConfigFromUserConfig
func CreateRubixConfigFromUserConfig(userConfig types.UserConfig, nodeDir string) (types.RubixConfig, error) {
    var rubixConfig types.RubixConfig
    rubixConfig.NodeDir = nodeDir
    // network dir = nodeDir / "mainnet"|"testnet"|"localnet"
    rubixConfig.NetworkDir = filepath.Join(nodeDir, networkDirName)
    rubixConfig.DidDir = filepath.Join(rubixConfig.NetworkDir, "dids")
    rubixConfig.TrustedNetwork = userConfig.Core.EnableTrustedNetwork
    // Ports: base + NodeIndex
    rubixConfig.PortConfig.IPFSPort = constants.IPFSPort + uint16(userConfig.Core.NodeIndex)
    // ... all other ports similarly
    rubixConfig.DBConfig.Port = int(constants.PostgresBasePort) + int(userConfig.Core.NodeIndex)
    // DB fields
    rubixConfig.DBConfig.DBName = userConfig.Db.DBName
    rubixConfig.DBConfig.Host = userConfig.Db.Host
    rubixConfig.DBConfig.Password = userConfig.Db.Password
    rubixConfig.DBConfig.Username = userConfig.Db.Username
    // DB pool params
    rubixConfig.DBConfig.Params = userConfig.Db.Params
    return rubixConfig, nil
}
```

### NewCore Constructor

```go
// core/core.go:151-257 — NewCore (abbreviated to essentials)
func NewCore(cfg *types.RubixConfig, log logger.Logger,
    networkMode string, defaultSetup bool, publishTokenChainDetails bool,
    fullNode bool, faucetURL string,
) (*Core, error) {
    c := &Core{
        cfg:           cfg,
        quorumRequest: make(map[string]*ConsensusStatus),
        pd:            make(map[string]*PledgeDetails),
        webReq:        make(map[string]*did.DIDChan),
        qc:            make(map[string]types.DIDCrypto),
        pqc:           make(map[string]types.DIDCrypto),
        secret:        util.GetRandBytes(32),
        defaultSetup:  defaultSetup,
        fullNode:      fullNode,
        faucetURL:     faucetURL,
        Ctx:           context.Background(),
    }

    // Set network mode
    switch networkMode {
    case constants.NetworkMode_Testnet:  c.testnet = true
    case constants.NetworkMode_Mainnet:  c.mainnet = true
    case constants.NetworkMode_Localnet: c.localnet = true
    }

    c.log = log.Named("Core")

    // --- DB SETUP ---
    dbOpts := storage.DBOpts{
        MaxConns:                  c.cfg.DBConfig.Params.MaxConnections,
        MinConns:                  c.cfg.DBConfig.Params.MinConnections,
        MaxConnLifetimeInSeconds:  time.Duration(c.cfg.DBConfig.Params.MaxConnectionLifetimeSeconds) * time.Second,
        MaxConnIdleTimeInSeconds:  time.Duration(c.cfg.DBConfig.Params.MaxConnectionIdletimeSeconds) * time.Second,
        StatementTimeoutInSeconds: time.Duration(c.cfg.DBConfig.Params.StatementTimeoutSeconds) * time.Second,
    }

    rubixDB, err := storage.NewRubixDB(c.Ctx, &c.cfg.DBConfig, dbOpts)
    // ... error check

    // --- WALLET SETUP ---
    c.w, err = wallet.NewWallet(c.Ctx, rubixDB, c.log)
    // ... error check

    // ... performance tracker, token sync manager, etc.
    return c, nil
}
```

### Storage Layer (pgxpool)

```go
// core/storage/storage.go:13-15
type RubixDB struct {
    pool *pgxpool.Pool
}

// core/storage/storage.go:17-26
func GetRubixDBConnectionString(dbConfig *types.DBConfig) string {
    return fmt.Sprintf(
        "host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
        dbConfig.Host, dbConfig.Port, dbConfig.Username, dbConfig.Password, dbConfig.DBName,
    )
}

// core/storage/storage.go:37-69
func NewRubixDB(ctx context.Context, dbConfig *types.DBConfig, opts DBOpts) (*RubixDB, error) {
    connStr := GetRubixDBConnectionString(dbConfig)
    config, err := pgxpool.ParseConfig(connStr)
    config.MaxConns = int32(opts.MaxConns)
    config.MinConns = int32(opts.MinConns)
    config.MaxConnLifetime = opts.MaxConnLifetimeInSeconds
    config.MaxConnIdleTime = opts.MaxConnIdleTimeInSeconds
    if opts.StatementTimeoutInSeconds > 0 {
        config.ConnConfig.RuntimeParams["statement_timeout"] =
            fmt.Sprintf("%d", opts.StatementTimeoutInSeconds.Milliseconds())
    }
    pool, err := pgxpool.NewWithConfig(ctx, config)
    if err := pool.Ping(ctx); err != nil { pool.Close(); return nil, err }
    return &RubixDB{pool: pool}, nil
}

func (r *RubixDB) Pool() *pgxpool.Pool { return r.pool }
func (r *RubixDB) BeginTx(ctx context.Context) (pgx.Tx, error) { return r.pool.Begin(ctx) }
```

### Wallet Constructor

```go
// core/wallet/wallet.go:14-20
type Wallet struct {
    ipfs    *ipfsnode.Shell
    ipfsOps types.IPFSOperations
    log     logger.Logger
    db      *storage.RubixDB
    Ctx     context.Context
}

// core/wallet/wallet.go:27-45
func NewWallet(ctx context.Context, db *storage.RubixDB, log logger.Logger) (*Wallet, error) {
    w := &Wallet{log: log.Named("wallet"), db: db, Ctx: ctx}
    err := w.db.InitSchema(w.Ctx)  // CREATE TABLE IF NOT EXISTS ...
    err = w.addProtocolValuesToLookupTables()  // seeds did_algo, token_role, token_type
    return w, nil
}
```

---

## 2. Config Types and Minimal config.toml

### Config Structs

```go
// types/db.go:5-9
type CoreConfig struct {
    NodeIndex            int    `toml:"node_index"`
    NetworkMode          string `toml:"network_mode"`
    EnableTrustedNetwork bool   `toml:"enable_trusted_network"`
}

// types/db.go:11-17
type DBParams struct {
    MaxConnections               int `toml:"max_connections"`
    MinConnections               int `toml:"min_connections"`
    MaxConnectionLifetimeSeconds int `toml:"max_connection_lifetime_seconds"`
    MaxConnectionIdletimeSeconds int `toml:"max_connection_idletime_seconds"`
    StatementTimeoutSeconds      int `toml:"statement_timeout_seconds"`
}

// types/db.go:19-26
type DBConfig struct {
    Host     string   `toml:"host"`
    Port     int      `toml:"-"`         // computed: PostgresBasePort + NodeIndex
    Username string   `toml:"username"`
    Password string   `toml:"password"`
    DBName   string   `toml:"db_name"`
    Params   DBParams `toml:"config"`
}

// types/db.go:34-38
type UserConfig struct {
    Core CoreConfig     `toml:"core"`
    Db   DBConfig       `toml:"db"`
    Ipfs IPFSUserConfig `toml:"ipfs"`
}

// types/db.go:58-65 — PortConfig: SendPort, ReceiverPort, IPFSPort, SwarmPort, IPFSAPIPort, RubixServerPort (all uint16)
// Populated automatically by CreateRubixConfigFromUserConfig — not set in config.toml.

// types/db.go:80-100
type RubixConfig struct {
    CfgData            RubixCfgData
    NodeConfigDir      string
    NodeDir            string
    NetworkDir         string
    DidDir             string
    PortConfig         PortConfig
    UnpledgePoolConfig UnpledgePoolConfig
    IPFSRecoveryConfig IPFSRecoveryConfig
    DBConfig           DBConfig
    MainnetBootstrap   []string
    TestnetBootstrap   []string
    LocalnetBootStrap  []string
    AsyncFTResponse    bool
    TrustedNetwork     bool
    NodePort           int
}
```

### Minimal config.toml

```toml
# core/config/config.go:22-54 — template
[core]
node_index = 0
network_mode = "localnet"
enable_trusted_network = false

[db]
host = "localhost"
username = "rubix"
password = "rubixpass"
db_name = "rubix"

[db.config]
max_connections = 50
min_connections = 5
max_connection_lifetime_seconds = 1
max_connection_idletime_seconds = 1
statement_timeout_seconds = 5

[ipfs]
mainnet_bootstrap_nodes = []
testnet_bootstrap_nodes = []
localnet_bootstrap_nodes = []
```

Note: `DBConfig.Port` is NOT in config.toml — it is computed as `PostgresBasePort (5433) + NodeIndex`. Node 0 connects to port 5433.

---

## 3. Token Creation (Genesis Path)

### generateTestTokens Flow

```go
// core/token.go:194-330
func (c *Core) generateTestTokens(reqID string, num int, did string, startIndex int) error {
    if !c.localnet { return fmt.Errorf("...") }

    dc, err := c.SetupDID(reqID, did)  // needs a web request channel
    currentTokenNumber, err := c.w.GetLocalTokenNumber()

    for globalIndex := startTokenNumber; globalIndex < finalTokenNumber; globalIndex++ {
        tokenLevel, numInLevel, _ := token.GetTokenLevelAndNumberForGlobalIndex(globalIndex)
        id, _ := c.getTokenIDForLocalTestTokens(tokenLevel, numInLevel) // e.g. "10000_1"

        // Build TransactionInfo
        txInfo := &models.TransactionInfo{
            Initiator: did, Owner: did,
            Epoch:   int(time.Now().Unix()),
            Network: constants.NetworkID_RBT_Local,  // "local"
            Tokens: &models.TransactionTokens{
                RBT: []*models.TokenInfo{{TokenID: id, PreviousTransactionID: ""}},
            },
        }

        // Serialize -> Sign -> Compute txID
        infoBytes, _ := models.SerializeTransactionInfo(txInfo)
        signatureBytes, _ := dc.PvtSign(infoBytes)
        sigStruct := &models.Signature{InitiatorSignature: hex.EncodeToString(signatureBytes)}
        sigBytes, _ := json.Marshal(sigStruct)
        txID, _ := wallet.ComputeTransactionID(txInfo) // SHA3-256 hex

        // IPFS pin (requires IPFS running)
        tokenHash, _ := c.w.Add(bytes.NewBufferString(id), did, constants.TokenProviderFunc_Add, true)
        c.w.Pin(tokenHash, constants.TokenProviderRole_Owner, did, "NA", did, "NA", 1.0)

        // Persist atomically
        mintRoleID := int16(models.GetTokenRoleID(constants.TokenRole_Mint))   // 1
        tokenTypeID := int16(models.GetTokenTypeID(constants.TokenType_RBT))   // 1
        c.w.PersistGenesisTokenRecord(
            &models.Transactions{ID: txID, Info: infoBytes, Signature: json.RawMessage(sigBytes)},
            &models.Token{
                TokenID: id, DID: did, TokenValue: 1.0,
                TokenStatus: int16(constants.TokenStatus_Free),  // 0
                TransactionID: txID, TokenStateHash: tokenHash,
                TokenType: tokenTypeID, LatestPosition: 0, LatestRole: mintRoleID,
            },
            &models.TokenChain{
                TokenID: id, TransactionID: txID,
                PreviousTransactionID: nil, Role: mintRoleID, Position: 0,
            },
        )
    }
}
```

### Token ID Generation

```go
// core/token.go:189-192 — local test tokens (faucet variant is identical fmt.Sprintf pattern)
func (c *Core) getTokenIDForLocalTestTokens(tokenLevel int, tokenNumber int) (string, error) {
    return strconv.Itoa(tokenLevel) + "_" + strconv.Itoa(tokenNumber), nil
    // e.g. "10000_1", "10000_2", ..., "10001_1"
}
```

### SerializeTransactionInfo

```go
// types/models/transaction_info.go:102-104
func SerializeTransactionInfo(txInfo *TransactionInfo) ([]byte, error) {
    return json.Marshal(txInfo)
}
```

### ComputeTransactionID

```go
// core/wallet/post_consensus_persistence.go:180-188
func ComputeTransactionID(txInfo *models.TransactionInfo) (string, error) {
    txInfoBytes, err := models.SerializeTransactionInfo(txInfo)  // json.Marshal
    hash := sha3.Sum256(txInfoBytes)
    return hex.EncodeToString(hash[:]), nil
}
```

### PersistGenesisTokenRecord (atomic 3-insert)

```go
// core/wallet/token_chain.go:91-138
func (w *Wallet) PersistGenesisTokenRecord(
    txRecord *models.Transactions,
    token *models.Token,
    entry *models.TokenChain,
) error {
    tx, err := w.BeginTx(w.Ctx)
    defer tx.Rollback(w.Ctx)

    // 1. Insert transaction (ON CONFLICT DO NOTHING)
    tx.Exec(w.Ctx,
        `INSERT INTO transactions (id, info, signature, created_at, updated_at)
         VALUES ($1, $2, $3, NOW(), NOW()) ON CONFLICT (id) DO NOTHING`,
        txRecord.ID, txRecord.Info, txRecord.Signature)

    // 2. Insert/upsert token
    tx.Exec(w.Ctx,
        `INSERT INTO tokens (token_id, parent_token_id, token_value, token_status, did, transaction_id,
         token_state_hash, token_type, latest_position, latest_role, created_at, updated_at)
         VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())
         ON CONFLICT (token_id) DO UPDATE SET
           transaction_id=EXCLUDED.transaction_id, token_state_hash=EXCLUDED.token_state_hash,
           latest_position=EXCLUDED.latest_position, latest_role=EXCLUDED.latest_role, updated_at=NOW()`,
        token.TokenID, token.ParentTokenID, token.TokenValue, token.TokenStatus,
        token.DID, token.TransactionID, token.TokenStateHash, token.TokenType,
        token.LatestPosition, token.LatestRole)

    // 3. Insert tokenchain (ON CONFLICT DO NOTHING)
    tx.Exec(w.Ctx,
        `INSERT INTO tokenchain (token_id, transaction_id, previous_transaction_id, role, position, created_at, updated_at)
         VALUES ($1, $2, $3, $4, $5, NOW(), NOW()) ON CONFLICT (token_id, position) DO NOTHING`,
        entry.TokenID, entry.TransactionID, entry.PreviousTransactionID, entry.Role, entry.Position)

    return tx.Commit(w.Ctx)
}
```

---

## 4. Token Read and Lock Paths

### GetFreeRBTTokens

```go
// core/wallet/token.go:29-64
func (w *Wallet) GetFreeRBTTokens(ownerDid string) ([]models.Token, []string, error) {
    rows, err := w.db.Pool().Query(w.Ctx,
        `SELECT token_id, parent_token_id, token_value, token_status, did, transaction_id,
         token_state_hash, token_type, latest_position, latest_role, created_at, updated_at
         FROM tokens WHERE token_type = (SELECT id FROM token_type WHERE name = $1)
         AND did = $2 AND token_status = 0`,
        constants.TokenType_RBT, ownerDid)
    // scans into []models.Token, returns (tokens, tokenIDs, error)
}
```

### ReadToken (single token by ID)

```go
// core/wallet/token.go:237-255
func (w *Wallet) ReadToken(tokenID string) (*models.Token, error) {
    row := w.db.Pool().QueryRow(w.Ctx,
        `SELECT token_id, parent_token_id, token_value, token_status, did, transaction_id,
         token_state_hash, token_type, latest_position, latest_role, created_at, updated_at
         FROM tokens WHERE token_id=$1`, tokenID)
    // scans into models.Token, returns (*Token, error)
}
```

### LockTokensByID / LockTokens

```go
// core/wallet/token.go:110-143
func (w *Wallet) LockTokensByID(tokenIDs []string) error {
    w.db.Pool().Exec(w.Ctx,
        `UPDATE tokens SET token_status=$1 WHERE token_id = ANY($2)`,
        constants.TokenStatus_Locked, tokenIDs)
}
// LockTokens(tokens []models.Token) extracts IDs, then runs the same UPDATE
```

### QueryAndLockByIDs (SELECT FOR UPDATE within tx)

```go
// core/wallet/token_lock.go:56-98
func (w *Wallet) QueryAndLockByIDs(ctx context.Context, tx pgx.Tx, ownerDID string,
    tokenIDs []string, tokenTypeName string) ([]models.Token, error) {
    rows, err := tx.Query(ctx, `
        SELECT t.token_id, t.parent_token_id, t.token_value, t.token_status,
               t.did, t.transaction_id, t.token_state_hash, t.token_type,
               t.latest_position, t.latest_role, t.created_at, t.updated_at
        FROM tokens t
        WHERE t.token_id = ANY($1::text[])
          AND t.did = $2
          AND t.token_type = (SELECT id FROM token_type WHERE name = $3)
          AND t.token_status = $4
        ORDER BY t.token_id
        FOR UPDATE OF t`, tokenIDs, ownerDID, tokenTypeName, constants.TokenStatus_Free)
    // collects rows, validates all found, returns ([]models.Token, error)
}
```

### lockTokensByIDs (self-contained: begin tx + lock + status update + commit)

```go
// core/wallet/token_lock.go:107-142
func (w *Wallet) lockTokensByIDs(ctx context.Context, ownerDID string,
    tokenIDs []string, tokenTypeName string, label string) ([]models.Token, error) {
    sort.Strings(tokenIDs)  // deadlock prevention
    tx, _ := w.db.BeginTx(ctx)
    defer tx.Rollback(ctx)
    tx.Exec(ctx, "SET LOCAL lock_timeout = '5s'")
    locked, _ := w.QueryAndLockByIDs(ctx, tx, ownerDID, tokenIDs, tokenTypeName)
    tx.Exec(ctx, `UPDATE tokens SET token_status = $1, updated_at = $2 WHERE token_id = ANY($3::text[])`,
        constants.TokenStatus_Locked, time.Now(), tokenIDs)
    tx.Commit(ctx)
    return locked, nil
}
```

### GetTokensFromDenomMap (RBT denomination selection)

```go
// core/wallet/token.go:203-235
func (w *Wallet) GetTokensFromDenomMap(denomMap map[types.DenomValue]types.DenomCount, did string) ([]models.Token, error) {
    for denomValue, denomCount := range denomMap {
        rows, _ := w.db.Pool().Query(w.Ctx,
            `SELECT ... FROM tokens WHERE token_value=$1 AND did=$2 AND token_status=$3 LIMIT $4`,
            denomValue, did, constants.TokenStatus_Free, denomCount)
        // appends to tokens
    }
}
```

---

## 5. Model Types

### Token

```go
// types/models/models.go:54-68
type Token struct {
    TokenID        string      `db:"token_id"`
    ParentTokenID  pgtype.Text `db:"parent_token_id"`
    TokenValue     float64     `db:"token_value"`
    TokenStatus    int16       `db:"token_status"`
    DID            string      `db:"did"`
    TransactionID  string      `db:"transaction_id"`
    TokenStateHash string      `db:"token_state_hash"`
    TokenType      int16       `db:"token_type"`
    LatestPosition int64       `db:"latest_position"`
    LatestRole     int16       `db:"latest_role"`
    CreatedAt      time.Time   `db:"created_at"`
    UpdatedAt      time.Time   `db:"updated_at"`
    SyncStatus     int         `db:"-"` // transient, not persisted
}
```

### Transactions

```go
// types/models/models.go:10-16
type Transactions struct {
    ID        string          `db:"id"`
    Info      json.RawMessage `db:"info"`
    Signature json.RawMessage `db:"signature"`
    CreatedAt time.Time       `db:"created_at"`
    UpdatedAt time.Time       `db:"updated_at"`
}
```

### TokenChain

```go
// types/models/models.go:18-27
type TokenChain struct {
    ID                    int32     `db:"id"`
    TokenID               string    `db:"token_id"`
    TransactionID         string    `db:"transaction_id"`
    PreviousTransactionID *string   `db:"previous_transaction_id"`  // nil for genesis
    Role                  int16     `db:"role"`
    Position              int64     `db:"position"`
    CreatedAt             time.Time `db:"created_at"`
    UpdatedAt             time.Time `db:"updated_at"`
}
```

### TransactionInfo (JSON stored in transactions.info)

```go
// types/models/transaction_info.go:5-14
type TransactionInfo struct {
    Initiator       string             `json:"initiator"`
    Owner           string             `json:"owner"`
    Epoch           int                `json:"epoch"`
    Network         string             `json:"network"`
    Tokens          *TransactionTokens `json:"tokens"`
    CommittedTokens []*TokenInfo       `json:"committedTokens"`
    Quorums         []*QuorumInfo      `json:"quorums"`
    Memo            string             `json:"memo"`
}

type TransactionTokens struct {
    RBT           []*TokenInfo `json:"rbt"`
    NFT           []*TokenInfo `json:"nft"`
    FT            []*TokenInfo `json:"ft"`
    SmartContract []*TokenInfo `json:"smartContract"`
}

type TokenInfo struct {
    TokenID               string  `json:"tokenId"`
    PreviousTransactionID string  `json:"previousTransactionID"`
    Data                  string  `json:"data"`
    TokenValue            float64 `json:"tokenValue"`
    DID                   string  `json:"did"`
}

type Signature struct {
    InitiatorSignature string            `json:"initiatorSignature"`
    Quorums            []QuorumSignature `json:"quorums"`
}
```

---

## 6. Schema and FK Order

### Key Table DDL

```sql
-- core/storage/schema.go

CREATE TABLE IF NOT EXISTS transactions (
    id          TEXT PRIMARY KEY,
    info        JSON NOT NULL,
    signature   JSONB NOT NULL,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS tokens (
    token_id         TEXT PRIMARY KEY,
    parent_token_id  TEXT REFERENCES tokens(token_id) ON DELETE SET NULL,
    token_value      NUMERIC NOT NULL CHECK (token_value >= 0),
    token_status     SMALLINT NOT NULL DEFAULT 99,
    did              TEXT NOT NULL,
    transaction_id   TEXT NOT NULL,
    token_state_hash TEXT NOT NULL,
    token_type       SMALLINT NOT NULL,
    latest_position  BIGINT NOT NULL DEFAULT 0,
    latest_role      SMALLINT,
    created_at       TIMESTAMPTZ DEFAULT NOW(),
    updated_at       TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT tokens_did_fk FOREIGN KEY (did) REFERENCES dids(did),
    CONSTRAINT transaction_id_fk FOREIGN KEY (transaction_id) REFERENCES transactions(id) DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT token_type_fk FOREIGN KEY (token_type) REFERENCES token_type(id)
);

CREATE TABLE IF NOT EXISTS tokenchain (
    id             INT GENERATED ALWAYS AS IDENTITY,
    token_id       TEXT NOT NULL,
    transaction_id TEXT NOT NULL,
    previous_transaction_id TEXT,
    role           SMALLINT NOT NULL,
    position       BIGINT NOT NULL,
    created_at     TIMESTAMPTZ DEFAULT NOW(),
    updated_at     TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (token_id, position),
    -- FKs to transactions(id), token_role(id), tokens(token_id)
);

-- Lookup tables
CREATE TABLE IF NOT EXISTS token_role (id SMALLINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY, name TEXT NOT NULL UNIQUE, is_active BOOLEAN DEFAULT TRUE);
CREATE TABLE IF NOT EXISTS token_type (id SMALLINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY, name TEXT NOT NULL UNIQUE, is_active BOOLEAN DEFAULT TRUE);
CREATE TABLE IF NOT EXISTS did_algo   (id SMALLINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY, name TEXT NOT NULL UNIQUE, is_active BOOLEAN DEFAULT TRUE);
CREATE TABLE IF NOT EXISTS dids       (did TEXT PRIMARY KEY, peer_id TEXT, local BOOLEAN DEFAULT TRUE, algo_id SMALLINT);
```

### FK Dependency Insertion Order

```
1. did_algo    -- seeded automatically by NewWallet
2. token_role  -- seeded automatically by NewWallet
3. token_type  -- seeded automatically by NewWallet
4. dids        -- must exist before tokens.did FK; insert your DID here manually
5. transactions -- must exist before tokens.transaction_id FK (DEFERRABLE INITIALLY DEFERRED)
6. tokens      -- references dids.did, transactions.id, token_type.id
7. tokenchain  -- references tokens.token_id, transactions.id, token_role.id
```

Gotcha: `transactions.id` FK on `tokens` is `DEFERRABLE INITIALLY DEFERRED` — within a single transaction you can insert tokens before transactions, but by commit time the FK must resolve. `PersistGenesisTokenRecord` inserts transactions first (step 1) then tokens (step 2) to be explicit.

---

## 7. Constants Quick Reference

### TokenStatus Values

```go
// constants/constants.go
const (
    TokenStatus_Free            = 0   // iota
    TokenStatus_Locked          = 1
    TokenStatus_Generated       = 2
    TokenStatus_Fetched         = 3
    TokenStatus_Transferred     = 4
    TokenStatus_Committed       = 5
    TokenStatus_Pledged         = 6
    TokenStatus_QuorumPledged   = 7
    TokenStatus_Burnt           = 8
    TokenStatus_BurntForFT      = 9
    TokenStatus_Deployed        = 10
    TokenStatus_Executed        = 11
    TokenStatus_PinnedAsService = 12
    TokenStatus_Orphaned        = 13
    TokenStatus_ChainSyncIssue  = 14
    TokenStatus_BeingDoubleSpent= 15
    TokenStatus_Seed            = 99
)
```

### NetworkID Values

```go
// constants/constants.go
const (
    NetworkID_RBT_Mainnet = "premint"
    NetworkID_RBT_Testnet = "testrbt"
    NetworkID_RBT_Local   = "local"
)
```

### Port Constants

```go
// constants/ipfs.go
const (
    PostgresBasePort uint16 = 5433   // DB port = 5433 + NodeIndex
    RubixServerPort  uint16 = 20000
)
```

### Lookup Table Arrays (IDs are positional — order matters)

```go
// types/models/lookup.go — canonical arrays (IDENTITY IDs = array position + 1)
var TokenRoleTypes = []TokenRole{
    {Name: "mint"},      // ID=1
    {Name: "transfer"},  // ID=2
    {Name: "execute"},   // ID=3
    {Name: "deploy"},    // ID=4
    {Name: "burn"},      // ID=5
    {Name: "commit"},    // ID=6
    {Name: "uncommit"},  // ID=7
    {Name: "pledge"},    // ID=8
    {Name: "unpledge"},  // ID=9
}

var TokenTypeTypes = []TokenType{
    {Name: "rbt"},            // ID=1
    {Name: "nft"},            // ID=2
    {Name: "ft"},             // ID=3
    {Name: "smart_contract"}, // ID=4
}

// Helper functions
func GetTokenRoleID(tokenRole string) int  // returns 1-based index, -1 if not found
func GetTokenTypeID(tokenType string) int  // returns 1-based index, -1 if not found
```

---

## 8. Bypass Notes

### Creating Tokens Without IPFS

`generateTestTokens` calls `c.w.Add()` and `c.w.Pin()` which require IPFS. To bypass: skip those calls, use a synthetic `TokenStateHash` (any non-empty string), and call `w.PersistGenesisTokenRecord()` directly. Prerequisite: the DID must already exist in the `dids` table (FK on `tokens.did`).

```go
// Direct wallet call — no Core, no IPFS, no DID signing required
txID := "your-tx-id-hex"
infoBytes, _ := json.Marshal(map[string]interface{}{"initiator": did, "owner": did, "epoch": time.Now().Unix(), "network": "local"})
sigBytes, _ := json.Marshal(map[string]string{"initiatorSignature": "test-sig"})
w.PersistGenesisTokenRecord(
    &models.Transactions{ID: txID, Info: json.RawMessage(infoBytes), Signature: json.RawMessage(sigBytes)},
    &models.Token{
        TokenID: "10000_1", DID: did, TokenValue: 1.0,
        TokenStatus: int16(constants.TokenStatus_Free),
        TransactionID: txID, TokenStateHash: "synthetic-hash-10000_1",
        TokenType: int16(models.GetTokenTypeID(constants.TokenType_RBT)),   // 1
        LatestPosition: 0, LatestRole: int16(models.GetTokenRoleID(constants.TokenRole_Mint)), // 1
    },
    &models.TokenChain{TokenID: "10000_1", TransactionID: txID, PreviousTransactionID: nil, Role: 1, Position: 0},
)
```

### Bypassing DID Signing

`dc.PvtSign(infoBytes)` requires DID key files in `DidDir/<did>/`. The DB only requires `info JSON NOT NULL` and `signature JSONB NOT NULL` — no cryptographic validation at the schema level. Pass any valid `json.RawMessage` for both fields in test records.

---

## 9. PersistPostConsensus (Non-Genesis Path)

For full consensus transactions (transfer, pledge, deploy, execute), use `PersistPostConsensus` instead of `PersistGenesisTokenRecord`.

```go
// core/wallet/post_consensus_persistence.go:25-34
type PostConsensusPersistenceRequest struct {
    Transaction     *models.Transactions
    TransactionInfo *models.TransactionInfo
    Signature       *models.Signature
    DID             string
    ExecutionRole   string          // "initiator" | "quorum" | "receiver"
    AffectedTokens  []string
    TokenChainRows  []models.TokenChain
    TokenStates     []models.Token
}

// core/wallet/post_consensus_persistence.go:48-50
func (w *Wallet) PersistPostConsensus(ctx context.Context, req *PostConsensusPersistenceRequest) error {
    return NewPostConsensusPersistenceCoordinator(w).Persist(ctx, req)
}
```

Persist() internal flow:

```go
// core/wallet/post_consensus_persistence.go:52-108
// 1. buildTransactionRecord — computes txID from TransactionInfo if not provided
// 2. If TokenChainRows/TokenStates empty -> BuildPersistencePayload() derives them
// 3. BeginTx
// 4. insertTransaction (ON CONFLICT DO NOTHING)
// 5. insertTransactionUnit (did + execution_role)
// 6. insertTokenChainRows (batch)
// 7. syncTokenChainIndex
// 8. upsertTokenStates
// 9. Commit
```

If `TokenChainRows` and `TokenStates` are pre-built and populated in the request, step 2 is skipped — the caller controls exactly what gets written.
