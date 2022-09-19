package core

import (
	"fmt"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/config"
	"github.com/rubixchain/rubixgoplatform/core/model"
)

const (
	MinQuorumRequired    int = 7
	MinConsensusRequired int = 5
)

type ConensusRequest struct {
	ReqID string `json:"req_id"`
	Type  int    `json:"type"`
	Mode  int    `json:"mode"`
}

type ConsensusResult struct {
	RunningCount int
	SuccessCount int
	FailedCount  int
}

type ConsensusStatus struct {
	AlphaResult ConsensusResult
	BetaResult  ConsensusResult
	GammaResult ConsensusResult
}

func (c *Core) GetAllQuorum() *model.QuorumList {
	var ql model.QuorumList
	for _, a := range c.cfg.CfgData.QuorumList.Alpha {
		if ql.Quorum == nil {
			ql.Quorum = make([]model.Quorum, 0)
		}
		q := model.Quorum{
			Type:    model.AlphaType,
			Address: a,
		}
		ql.Quorum = append(ql.Quorum, q)
	}
	for _, a := range c.cfg.CfgData.QuorumList.Beta {
		if ql.Quorum == nil {
			ql.Quorum = make([]model.Quorum, 0)
		}
		q := model.Quorum{
			Type:    model.BetaType,
			Address: a,
		}
		ql.Quorum = append(ql.Quorum, q)
	}
	for _, a := range c.cfg.CfgData.QuorumList.Gamma {
		if ql.Quorum == nil {
			ql.Quorum = make([]model.Quorum, 0)
		}
		q := model.Quorum{
			Type:    model.GammaType,
			Address: a,
		}
		ql.Quorum = append(ql.Quorum, q)
	}
	return &ql
}

func (c *Core) AddQuorum(ql *model.QuorumList) error {
	update := false
	for _, q := range ql.Quorum {
		switch q.Type {
		case model.AlphaType:
			update = true
			if c.cfg.CfgData.QuorumList.Alpha == nil {
				c.cfg.CfgData.QuorumList.Alpha = make([]string, 0)
			}
			c.cfg.CfgData.QuorumList.Alpha = append(c.cfg.CfgData.QuorumList.Alpha, q.Address)
		case model.BetaType:
			update = true
			if c.cfg.CfgData.QuorumList.Beta == nil {
				c.cfg.CfgData.QuorumList.Beta = make([]string, 0)
			}
			c.cfg.CfgData.QuorumList.Beta = append(c.cfg.CfgData.QuorumList.Beta, q.Address)
		case model.GammaType:
			update = true
			if c.cfg.CfgData.QuorumList.Gamma == nil {
				c.cfg.CfgData.QuorumList.Gamma = make([]string, 0)
			}
			c.cfg.CfgData.QuorumList.Gamma = append(c.cfg.CfgData.QuorumList.Gamma, q.Address)
		}
	}
	if update {
		return c.updateConfig()
	} else {
		return nil
	}
}

func (c *Core) getQuorumList(t int) *config.QuorumList {
	var ql config.QuorumList
	switch t {
	case 1:
		return nil
		// TODO :: Add method to get type1 quorurm
	case 2:
		ql = c.cfg.CfgData.QuorumList
	}

	if len(ql.Alpha) < MinQuorumRequired || len(ql.Beta) < MinQuorumRequired || len(ql.Gamma) < MinQuorumRequired {
		c.log.Error("Failed to get required quorur", "al", len(ql.Alpha), "bl", len(ql.Beta), "gl", len(ql.Gamma))
		return nil
	}
	return &ql
}

func (c *Core) initiateConsensus(cr ConensusRequest) error {
	cs := ConsensusStatus{
		AlphaResult: ConsensusResult{
			RunningCount: 0,
			SuccessCount: 0,
			FailedCount:  0,
		},
		BetaResult: ConsensusResult{
			RunningCount: 0,
			SuccessCount: 0,
			FailedCount:  0,
		},
		GammaResult: ConsensusResult{
			RunningCount: 0,
			SuccessCount: 0,
			FailedCount:  0,
		},
	}
	ql := c.getQuorumList(cr.Type)
	if ql == nil {
		c.log.Error("Failed to get required quorums")
		return fmt.Errorf("Failed to get required quorums")
	}
	c.qlock.Lock()
	c.quorumRequest[cr.ReqID] = &cs
	c.qlock.Unlock()

	for _, a := range ql.Alpha {
		go c.connectQuorum(cr, a, 0)
	}
	for _, a := range ql.Beta {
		go c.connectQuorum(cr, a, 1)
	}
	for _, a := range ql.Gamma {
		go c.connectQuorum(cr, a, 2)
	}
	loop := true
	var err error
	err = nil
	for {
		time.Sleep(time.Second)
		c.lock.Lock()
		cs, ok := c.quorumRequest[cr.ReqID]
		if !ok {
			loop = false
			err = fmt.Errorf("Invalid request")
		} else {
			if cs.AlphaResult.SuccessCount >= MinConsensusRequired && cs.BetaResult.SuccessCount >= MinConsensusRequired && cs.GammaResult.SuccessCount >= MinConsensusRequired {
				loop = false
				c.log.Debug("Conensus finished successfully")
			} else if cs.AlphaResult.RunningCount == 0 && cs.BetaResult.RunningCount == 0 && cs.GammaResult.RunningCount == 0 {
				loop = false
				err = fmt.Errorf("Consensus failed")
				c.log.Error("Conensus failed")
			}
		}
		c.lock.Unlock()
		if !loop {
			break
		}
	}
	return err
}

func (c *Core) startConensus(id string, qt int) {
	c.qlock.Lock()
	defer c.qlock.Unlock()
	cs, ok := c.quorumRequest[id]
	if !ok {
		return
	}
	switch qt {
	case 0:
		cs.AlphaResult.RunningCount++
	case 1:
		cs.BetaResult.RunningCount++
	case 2:
		cs.GammaResult.RunningCount++
	}
}

func (c *Core) finishConensus(id string, qt int, status bool) {
	c.qlock.Lock()
	defer c.qlock.Unlock()
	cs, ok := c.quorumRequest[id]
	if !ok {
		return
	}
	switch qt {
	case 0:
		cs.AlphaResult.RunningCount--
		if status {
			cs.AlphaResult.SuccessCount++
		} else {
			cs.AlphaResult.FailedCount++
		}

	case 1:
		cs.BetaResult.RunningCount--
		if status {
			cs.BetaResult.SuccessCount++
		} else {
			cs.BetaResult.FailedCount++
		}
	case 2:
		cs.GammaResult.RunningCount--
		if status {
			cs.GammaResult.SuccessCount++
		} else {
			cs.GammaResult.FailedCount++
		}
	}
}

func (c *Core) connectQuorum(cr ConensusRequest, addr string, qt int) {
	c.startConensus(cr.ReqID, qt)
	p, err := c.getPeer(addr)
	if err != nil {
		c.log.Error("Faile to get peer connection", "err", err)
		c.finishConensus(cr.ReqID, qt, false)
		return
	}
	defer p.Close()
	c.finishConensus(cr.ReqID, qt, true)
}
