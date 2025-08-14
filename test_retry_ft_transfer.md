# Testing the Retry FT Transfer API

## API Endpoint
`POST /api/retry-ft-transfer`

## Request Format
```json
{
  "transaction_id": "uuid-of-original-transfer",
  "sender_did": "bafybmi...",
  "receiver_did": "bafybmi..."
}
```

## Response Format
```json
{
  "status": true,
  "message": "Successfully retried FT transfer for transaction {id}. Sent {count} {name} tokens to receiver.",
  "transaction_id": "uuid-of-original-transfer",
  "token_count": 10
}
```

## Test Cases

### 1. Valid Retry Request
```bash
curl -X POST http://localhost:20000/api/retry-ft-transfer \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "transaction_id": "valid-uuid",
    "sender_did": "sender-did",
    "receiver_did": "receiver-did"
  }'
```

### 2. Invalid Transaction ID
```bash
curl -X POST http://localhost:20000/api/retry-ft-transfer \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "transaction_id": "invalid-uuid",
    "sender_did": "sender-did",
    "receiver_did": "receiver-did"
  }'
```
Expected: "FT transaction not found or sender/receiver mismatch"

### 3. Missing Parameters
```bash
curl -X POST http://localhost:20000/api/retry-ft-transfer \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "transaction_id": "valid-uuid"
  }'
```
Expected: "sender_did is required"

## How It Works

1. **Transaction Retrieval from Explorer**: The API fetches transaction details from the explorer API:
   - Testnet: `https://testnet-app-api.rubixexplorer.com/api/Transaction/GetById/{transaction_id}`
   - Mainnet: `https://rexplorerapi.azurewebsites.net/api/Transaction/GetById/{transaction_id}`

2. **Transaction Validation**: Validates that:
   - Transaction type is "FT"
   - Sender DID matches the request
   - Receiver DID matches the request

3. **Token List Extraction**: Gets the FT token list from explorer response (`ftTokenList` field)

4. **Token Preparation**: For each token in the list, fetches the latest block information from local storage and prepares the TokenInfo array

5. **Receiver Connection**: Establishes connection with the receiver peer using their DID

6. **Token Transfer**: Sends the tokens to receiver using the existing `APISendFTToken` endpoint

7. **Response**: Returns success/failure status with details about the number of tokens sent

## Key Features

- **Explorer Integration**: Fetches transaction and token details from explorer API instead of local database
- **Automatic Environment Detection**: Uses appropriate explorer endpoint based on testnet/mainnet configuration
- **Transaction Validation**: Validates transaction type and participant DIDs from explorer data
- **Token Recovery**: Extracts complete token list from explorer's `ftTokenList` field
- **Local Block Verification**: Still uses local storage for token chain blocks to ensure integrity
- **Error handling**: Comprehensive error messages for various failure scenarios
- **Background updates**: Updates explorer balances after successful retry

## Implementation Files

1. **Core Logic**: `core/ft_retry_transfer.go`
   - `RetryFTTransfer()` - Main retry logic
   - `RetryFTTransferRequest` - Request structure
   - `RetryFTTransferResponse` - Response structure

2. **API Handler**: `server/ft.go`
   - `APIRetryFTTransfer()` - HTTP endpoint handler

3. **Route Registration**: 
   - `setup/setup.go` - API path constant
   - `server/server.go` - Route registration

## Security Considerations

- Validates sender DID has access before allowing retry
- Only allows retry for existing transactions
- Verifies sender and receiver DIDs match the original transaction
- Uses existing token transfer mechanisms ensuring consensus validation