package core
import(

	"fmt"
	"strings"
	"github.com/rubixchain/rubixgoplatform/core/storage"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

func NewMiningQuorumManager(s storage.Storage, log logger.Logger) (*QuorumManager, error) {
	qm := &QuorumManager{
		s:   s,
		log: log.Named("mining_quorum_manager"),
	}
	err := qm.s.Init(MiningQuorumStorage, &QuorumData{}, true)
	if err != nil {
		qm.log.Error("Failed to init quorum storage", "err", err)
		return nil, err
	}
	var qd []QuorumData
	err = qm.s.Read(MiningQuorumStorage, &qd, "type=?", QuorumTypeTwo)
	if err == nil {
		qm.ql = make([]string, 0)
		for _, q := range qd {
			// Node with version v0.0.17 or prior will have stored the addresses in
			// <peer ID>.<did> format. To make it compatible the current implementation,
			// we check if its in the prior format, and if its so, then we change it to
			// <did> format and update it in quorummanager table
			if isOldAddressFormat(q.Address) {
				quorumAddressElements := strings.Split(q.Address, ".")
				quorumDID := quorumAddressElements[1]

				// Replace the old address format with new format in quorummanager
				var updatedQuorumDetails QuorumData = QuorumData{
					Type:    q.Type,
					Address: quorumDID,
				}
				err = qm.s.Write(MiningQuorumStorage, &updatedQuorumDetails)
				if err != nil {
					return nil, fmt.Errorf("failed while writing quorum info with new address format in quorummanager table, err: %v", err)
				}

				err := qm.s.Delete(MiningQuorumStorage, &QuorumData{}, "address=?", q.Address)
				if err != nil {
					return nil, fmt.Errorf("failed while deleting quorum info to replace with new address format in quorummanager table, err: %v", err)
				}

				qm.ql = append(qm.ql, quorumDID)
			} else {
				qm.ql = append(qm.ql, q.Address)
			}
		}
	}
	return qm, nil
}