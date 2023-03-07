package unpledge

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/EnsurityTechnologies/logger"
	"github.com/rubixchain/rubixgoplatform/core/wallet"

	"golang.org/x/crypto/sha3"
)

type UnPledge struct {
	log logger.Logger
}

type pledgedTokenIDs struct {
	TokenID string
}

func (un *UnPledge) listAllPledgedToken(did string) []pledgedTokenIDs {
	w := wallet.Wallet{}
	var u []pledgedTokenIDs
	pledgedtokenlist, err := w.GetAllPledgedTokens(did)
	if err != nil {
		un.log.Error("Failed to get tokens:, err")
	}
	fmt.Println(pledgedtokenlist)
	for i := range pledgedtokenlist {
		u = append(u, pledgedTokenIDs{
			TokenID: pledgedtokenlist[i].TokenID,
		})
	}
	return u
}

func (un *UnPledge) Lastblock(tokenID string) (string, string, error) {
	w := wallet.Wallet{}
	lastblock := w.GetLatestTokenBlock(tokenID)
	if lastblock == nil {
		un.log.Error("latest block for token %s not found", tokenID)
		return "", "", fmt.Errorf("latest block for token %s not found", tokenID)
	}
	ReceiverDID := lastblock.GetReceiverDID()
	transactionID := lastblock.GetTid()
	return ReceiverDID, transactionID, nil
}

func (un *UnPledge) concat(rdid string, pt string) (string, error) {
	if rdid == "" {
		err := errors.New("ReceiverDID cannot be empty")
		un.log.Error(err.Error())
		return "", err
	}
	if pt == "" {
		err := errors.New("PledgeTokenID cannot be empty")
		un.log.Error(err.Error())
		return "", err
	}
	valueConcated := rdid + pt
	return valueConcated, nil
}

func (un *UnPledge) difficultlevel(level int, lastvalue string) string {
	difflev := DiffLevel[level]
	result := lastvalue[len(lastvalue)-difflev:]
	return result
}

func (un *UnPledge) CalcSHA256Hash(value string) string {
	hash := sha256.Sum256([]byte(value))
	hashed := fmt.Sprintf("%x", hash)
	fmt.Printf("Sha3-256 value is %x", hashed)
	return hashed
}

func (un *UnPledge) proofcreation(did string) ([]string, error) {
	pledgedTokens := un.listAllPledgedToken(did)
	var proof []string
	for i := range pledgedTokens {
		tokenID := pledgedTokens[i].TokenID            // Qm1
		ReciDID, transID, err := un.Lastblock(tokenID) // receiver DID and Transaction ID
		if err != nil {
			un.log.Error("error getting last block for token %s: %s", tokenID, err.Error())
			continue
		}
		un.log.Debug("Token is %s", tokenID)
		un.log.Debug("Receiver DID is %s", ReciDID)
		un.log.Debug("Transaction ID is %s", transID)

		value, err := un.concat(tokenID, ReciDID) // concat ReceiverDID and TokenID
		if err != nil {
			un.log.Error("error concatenating values: %s", err.Error())
			continue
		}
		ValueToBeHashed := un.CalcSHA256Hash(value) // value to be hashed
		val := un.difficultlevel(4, transID)
		hashLen := len(ValueToBeHashed)
		valLen := len(val)

		//fetch level

		for {
			count := 0
			// Compute the SHA3-256 hash of the string
			hash := sha3.Sum256([]byte(ValueToBeHashed))

			// Convert the hash to a hex-encoded string
			hashString := hex.EncodeToString(hash[:])
			proof = append(proof, hashString)

			// Check if the last n characters of the hash match the desired value
			if len(hashString) >= hashLen && valLen <= hashLen {
				hashSuffix := hashString[hashLen-valLen:]
				if hashSuffix == val {
					un.log.Debug("hashing completed at %d", count)

					// Append the hashed value to the proof array
					if count%1000 == 0 {
						proof = append(proof, hashString)
					}
					break
				}
			}
			// Update the string to be hashed in the next iteration of the loop
			count++
			ValueToBeHashed = hashString
		}
	}
	return proof, nil
}

func (un *UnPledge) proofverification(proof []string) error {
	var token string
	var recDID string
	if len(proof) < 1 {
		un.log.Error("Proof is empty")
	}

	valToBeHash, err := un.concat(recDID, token)

	if err != nil {
		un.log.Error("Unable to concat ReciDID and token ID")
	}

	hash := sha256.Sum256([]byte(valToBeHash))
	hashString := hex.EncodeToString(hash[:])

	firstlineProof := proof[0]

	if firstlineProof == "" {
		un.log.Error("Firstline of proof is empty")
	}
	if firstlineProof != string(hashString) {
		un.log.Error("Firstline of proof mismatch")
	}
	prooflen := len(proof[1:])
	l := prooflen / 2

	firstHalf := proof[1:l]
	secondHalf := proof[l+1:]
	//middle := proof[l]

	rand.Seed(time.Now().UnixNano())

	randIndexFH := rand.Intn(len(firstHalf))
	randIndexSH := rand.Intn(len(secondHalf))

	var randFH [32]byte
	var randSH [32]byte

	for i := 0; i < 1000; i++ {
		randFH = sha256.Sum256([]byte(firstHalf[randIndexFH]))
	}
	hashrandFH := hex.EncodeToString(randFH[:])

	for i := 0; i < 1000; i++ {
		randSH = sha256.Sum256([]byte(secondHalf[randIndexSH]))
	}
	hashrandSH := hex.EncodeToString(randSH[:])

	firstHalfHash := firstHalf[randIndexFH+1]
	secondHalfHash := secondHalf[randIndexSH+1]

	if hashrandFH != firstHalfHash || hashrandSH != secondHalfHash {
		un.log.Error("Unable to verify proof")
	}
	un.log.Debug("Proof Verified")
	return nil
}
