package core

import (
	"github.com/rubixchain/rubixgoplatform/core/storage"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

const (
	QuorumTypeOne int = iota + 1
	QuorumTypeTwo
)

const (
	QuorumStorage string = "quorummanager"
)

const (
	GenericIssue int = iota
	ParentTokenNotBurned
	TokenChainNotSynced
)

type QuorumManager struct {
	ql  []string
	s   storage.Storage
	log logger.Logger
}

type QuorumData struct {
	Type    int    `gorm:"column:type" json:"type"`
	Address string `gorm:"column:address;primaryKey" json:"address"`
}

func NewQuorumManager(s storage.Storage, log logger.Logger) (*QuorumManager, error) {
	qm := &QuorumManager{
		s:   s,
		log: log.Named("quorum_manager"),
	}
	err := qm.s.Init(QuorumStorage, &QuorumData{}, true)
	if err != nil {
		qm.log.Error("Failed to init quorum storage", "err", err)
		return nil, err
	}
	var qd []QuorumData
	err = qm.s.Read(QuorumStorage, &qd, "type=?", QuorumTypeTwo)
	if err == nil {
		qm.ql = make([]string, 0)
		for _, q := range qd {
			qm.ql = append(qm.ql, q.Address)
		}
	}
	return qm, nil
}

// GetQuorum will get the configured or available quorum
func (qm *QuorumManager) GetQuorum(t int, lastChar string) []string {
	//QuorumTypeOne is to select quorums from the public pool of quorums instead of a private subnet.
	//Once a new node is created, it will create a DID. Using the command "registerdid", the peerID and DID will be
	//published in the network, and all the nodes listening to the subscription will have the DID added on the DIDPeerTable
	//A new variable quorumList is created, which will contain all the nodes which has DID with same last character as Transaction ID.
	//It would throw an error if it cannot find any relevant data of if the number of nodes is less than 5.
	//Then a separate array of type String called quorumAddrList, which will simply contain the address of nodes i.e. PeerID.DID
	//"quorumAddrList" is returned and checking the availability of nodes would be done in initiateConsensus function in quorum_initiator.go.
	switch t {
	case QuorumTypeOne:
		var quorumList []wallet.DIDPeerMap
		err := qm.s.Read(wallet.DIDPeerStorage, &quorumList, "did_last_char=?", lastChar)
		if err != nil {
			qm.log.Error("Quorums not present")
			return nil
		}
		if len(quorumList) < 5 {
			qm.log.Error("Not enough quorums present")
			return nil
		}
		var quorumAddrList []string
		quorumCount := 0
		for _, q := range quorumList {
			addr := string(q.PeerID + "." + q.DID)
			quorumAddrList = append(quorumAddrList, addr)
			quorumCount = quorumCount + 1
			if quorumCount == 7 {
				break
			}
		}
		return quorumAddrList
	case QuorumTypeTwo:
		return qm.ql
	}
	return nil
}

func (qm *QuorumManager) AddQuorum(qds []QuorumData) error {
	str := make([]string, 0)
	for _, qd := range qds {
		err := qm.s.Write(QuorumStorage, &qd)
		if err != nil {
			qm.log.Error("Failed to write to quorum storage", "err", err)
			return err
		}
		str = append(str, qd.Address)
	}
	qm.ql = str
	return nil
}

func (qm *QuorumManager) RemoveAllQuorum(t int) error {
	err := qm.s.Delete(QuorumStorage, &QuorumData{}, "type=?", t)
	if err != nil {
		qm.log.Error("Failed to delete quorum data", "err", err)
	}
	return err
}
