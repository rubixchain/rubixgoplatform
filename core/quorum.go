package core

import (
	"github.com/EnsurityTechnologies/logger"
	"github.com/rubixchain/rubixgoplatform/core/storage"
)

const (
	QuorumTypeOne int = iota + 1
	QuorumTypeTwo
)

const (
	QuorumStorage string = "quorummanager"
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
func (qm *QuorumManager) GetQuorum(t int) []string {
	switch t {
	case QuorumTypeOne:
		return nil
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
