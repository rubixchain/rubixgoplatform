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

1. **Transaction Validation**: The API first checks if the transaction exists in `FTTransactionHistoryStorage` with matching sender and receiver DIDs.

2. **Token Retrieval**: It queries `FTTokenStorage` to get all tokens associated with the transaction ID.

3. **Token Preparation**: For each token, it fetches the latest block information and prepares the TokenInfo array.

4. **Receiver Connection**: Establishes connection with the receiver peer using their DID.

5. **Token Transfer**: Sends the tokens to receiver using the existing `APISendFTToken` endpoint.

6. **Response**: Returns success/failure status with details about the number of tokens sent.

## Key Features

- **Uses FT-specific storage**: Properly queries `FTTransactionHistoryStorage` instead of general transaction storage
- **Validates token count**: Checks if retrieved tokens match the expected count from history
- **Error handling**: Comprehensive error messages for various failure scenarios
- **Maintains consistency**: Uses original transaction's epoch and metadata
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