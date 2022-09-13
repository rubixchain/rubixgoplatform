package core

import "github.com/rubixchain/rubixgoplatform/core/config"

const (
	MinQuorumRequired    int = 7
	MinConsensusRequired int = 5
)

func (c *Core) getQuorumList(t int) *config.QuorumList {
	var ql config.QuorumList
	switch t {
	case 1:
		// TODO :: Add method to get type1 quorurm
	case 2:
		ql = c.cfg.CfgData.QuorumList
	}

	if len(ql.Alpha) < MinQuorumRequired || len(ql.Beta) < MinQuorumRequired || len(ql.Gamma) < MinQuorumRequired {
		return nil
	}
	return &ql
}
