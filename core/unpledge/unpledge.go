package unpledge

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/EnsurityTechnologies/logger"
	"github.com/rubixchain/rubixgoplatform/core/wallet"

	"golang.org/x/crypto/sha3"
)

type UnPledge struct {
	log logger.Logger
}

func (un *UnPledge) GetPledgedTokenID(did string) ([]string, error) {
	w := wallet.Wallet{}
	var tokenIDs []string
	pledgedTokens, err := w.GetAllPledgedTokens(did)
	if err != nil {
		return nil, err
	}
	for _, token := range pledgedTokens {
		tokenIDs = append(tokenIDs, token.TokenID)
	}
	return tokenIDs, nil
}

func (un *UnPledge) GetLastBlockReceiverDID(TokenID string) (string, error) {
	w := wallet.Wallet{}
	lastBlock := w.GetLatestTokenBlock(TokenID)
	if lastBlock == nil {
		un.log.Error("latest block for token %s not found", TokenID)
		return "", nil
	}
	ReceiverDID := lastBlock.GetReceiverDID()
	return ReceiverDID, nil
}

func (un *UnPledge) GetLastBlockTransactionID(TokenID string) (string, error) {
	w := wallet.Wallet{}
	lastBlock := w.GetLatestTokenBlock(TokenID)
	if lastBlock == nil {
		un.log.Error("latest block for token %s not found", TokenID)
		return "", nil
	}
	TransactionID := lastBlock.GetTid()
	return TransactionID, nil
}

func (un *UnPledge) Concat(TokenID string, ReceiverDID string) (string, error) {
	if TokenID == "" {
		err := errors.New("TokenID cannot be empty")
		un.log.Error(err.Error())
		return "", err
	}
	if ReceiverDID == "" {
		err := errors.New("ReceiverDID cannot be empty")
		un.log.Error(err.Error())
		return "", err
	}
	valueConcated := TokenID + ReceiverDID
	return valueConcated, nil
}

func (un *UnPledge) CalcSHA_256Hash(input string) string {
	hash := sha256.Sum256([]byte(input))
	hashedInput := fmt.Sprintf("%x", hash)
	return hashedInput
}

func (un *UnPledge) CalcSHA_3_256Hash(input string) string {
	hash := sha3.Sum256([]byte(input))
	hashedInput := fmt.Sprintf("%x", hash)
	return hashedInput
}

func (un *UnPledge) CalcSHA3_256Hash1000Times(input string) string {
	var hashedInput string
	for i := 0; i < 1000; i++ {
		hash := sha3.Sum256([]byte(input))
		hashedInput = fmt.Sprintf("%x", hash)
		input = hashedInput
	}
	return hashedInput
}

func (un *UnPledge) getCurrentLevel() int {
	url := "http://13.76.134.226:9090/getCurrentLevel"
	resp, err := http.Get(url)
	if err != nil {
		un.log.Error("Error:", err)
		return -1
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		un.log.Error("Error:", err)
		return -1
	}

	var data map[string]interface{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		un.log.Error("Error:", err)
		return -1
	}

	level := int(data["level"].(float64))
	un.log.Debug("Level:", level)
	return level
}
func (un *UnPledge) Difficultlevel() int {
	cl := un.getCurrentLevel()
	return DiffLevel[cl]
}
func (un *UnPledge) saveProofToFile(proof []string, filePath string) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)

	for _, line := range proof {
		_, err := writer.WriteString(line + "\n")
		if err != nil {
			return err
		}
	}

	err = writer.Flush()
	if err != nil {
		return err
	}

	return nil
}
func (un *UnPledge) proofcreation(did string) ([]string, error) {
	var proof []string
	tokenIDs, err := un.GetPledgedTokenID(did)
	if err != nil {
		un.log.Error("Error fetching Pledged Tokens ")
	}
	for i := range tokenIDs {
		TokenID := tokenIDs[i]
		ReceiverDID, err := un.GetLastBlockReceiverDID(TokenID)

		if err != nil {
			un.log.Error("Unable to fetch Receiver DID from last block")
			continue
		}

		TransactionID, err := un.GetLastBlockTransactionID(TokenID)
		if err != nil {
			un.log.Error("Unable to fetch Transaction ID from last block")
			continue
		}

		ValueToBeHashed, err := un.Concat(TokenID, ReceiverDID)
		if err != nil {
			un.log.Error("Unable Concat Receiver DID and Token ID")
		}

		dl := un.Difficultlevel()

		proof = append(proof, strconv.Itoa(dl))

		ValueHashed := un.CalcSHA_256Hash(ValueToBeHashed)
		proof = append(proof, ValueHashed)

		SuffixTransactionID := TransactionID[len(TransactionID)-dl:]

		count := 1

		for {

			targetHash := un.CalcSHA_3_256Hash(ValueHashed)

			if count%1000 == 0 {
				proof = append(proof, targetHash)
			}

			SuffixTarget := targetHash[len(ValueHashed)-dl:]

			if SuffixTarget == SuffixTransactionID {
				proof = append(proof, targetHash)

				un.log.Debug("Hashing completed at ", count)

				un.log.Debug("Proof hash is ", targetHash, " Transaction ID is ", TransactionID)

				break
			}

			count++
			ValueHashed = targetHash
		}

		pf := un.saveProofToFile(proof, tokenIDs[i]+".proof")
		if pf != nil {
			un.log.Error("Unable to save proof.json")
		}
	}
	return proof, nil
}

func (un *UnPledge) proofverification(tokenID string, proof []string) error {

	for {
		TokenID := tokenID
		ReceiverDID, err := un.GetLastBlockReceiverDID(TokenID)
		if err != nil {
			un.log.Error("Unable to fetch Receiver DID from last block")
			break
		}
		TransactionID, err := un.GetLastBlockTransactionID(TokenID)
		if err != nil {
			un.log.Error("Unable to fetch Transaction ID from last block")
		}

		ValueToBeHashed, err := un.Concat(TokenID, ReceiverDID)
		if err != nil {
			un.log.Error("Unable to verify proof")
			break
		}

		valueHashed := un.CalcSHA_256Hash(ValueToBeHashed)
		if proof[0] == "" {
			un.log.Error("Unable to verify proof")
			break
		}
		dl := un.Difficultlevel()
		if proof[0] != strconv.Itoa(dl) {
			un.log.Error("Unable to verify proof")
			break
		}

		if proof[1] != valueHashed {
			un.log.Error("Unable to verify proof")
			break
		}

		proofToVerify := proof[1:] // Exculding firstline (Difficuilty level)
		lenProof := len(proof)
		lenProoftoVerify := len(proofToVerify)
		l := lenProoftoVerify / 2

		firstHalf := proof[1 : l-1]
		secondHalf := proof[l+1 : lenProoftoVerify-1]

		rand.Seed(time.Now().UnixNano())

		randIndexInFH := rand.Intn(len(firstHalf) - 1)
		randIndexInSH := rand.Intn(len(secondHalf) - 1)

		RandomHashInFH := firstHalf[randIndexInFH]
		RandomHashInSH := secondHalf[randIndexInSH]

		TargetHashInFH := firstHalf[randIndexInFH+1]
		TargetHashInSH := secondHalf[randIndexInSH+1]

		if un.CalcSHA3_256Hash1000Times(RandomHashInFH) != TargetHashInFH || un.CalcSHA3_256Hash1000Times(RandomHashInSH) != TargetHashInSH {
			un.log.Error("Unable to verify proof")
			break
		}
		var c int
		counter := 0
		target := proof[lenProof-3]

		SuffixLasthash := TransactionID[len(TransactionID)-dl:]

		for {
			targetHash := un.CalcSHA_3_256Hash(target)
			SuffixTarget := targetHash[len(targetHash)-dl:]

			if SuffixTarget == SuffixLasthash {
				c = counter
				break
			}
			counter++
			target = targetHash
		}
		if c > 1000 {
			un.log.Error("Unable to verify proof")
			break
		} else {
			un.log.Debug("Proof Verified")
		}
		break
	}
	return nil
}
