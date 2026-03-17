package core

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/rubixchain/rubixgoplatform/util"
)

const (
	QuorumTypeOne int = iota + 1
	QuorumTypeTwo
)

// isOldAddressFormat checks if the address is in <peerID>.<did> format (followed in versions v0.0.17 and before)
func isOldAddressFormat(address string) bool {
	return len(strings.Split(address, ".")) == 2
}

// TODO: Alter the following once new testnet quorums are running
func (c *Core) AddDefaulTestnetQuorums() {
	var faucetQuorumList []string = []string{
		"12D3KooWAoKtpBgQzBmt8sGB8RSn4RwSL2ZDp5rFz5jsDqgUJRuQ.bafybmibeoj772f5bvkoljeymipgzu7p4j32j73tc4detm4wpc5hebolvd4",
		"12D3KooWD3ycXgTvx1s1nm9K3z3VAPnxvVCupA41EFDpDX2vhaNW.bafybmigemcjb6ivksuyiuf23geykag3tvw4jtuxqaesjpggrlnujmowx2i",
		"12D3KooW9ukxuhMEE3jhyvdVC64Z1MbWUYYCpPiBv9uXCnW4wwHB.bafybmid6gcm6dcubsacyxpg7nmmpzo7czia5cs57s5l2xtn364ijqgqwhe",
		"12D3KooWEA9Kko7YabDWdFzzVru6fZTfwUU7sBRUYhUeyPY3UP9B.bafybmicmngm6twtypkwebnzubwx6k2zl2r7inao3vhxjdl7c5mqa2avezm",
		"12D3KooWHwDTwNCowHaJWZoDj3CtzhS6irAjyx3HVKLvFeZRwudL.bafybmihnveuzhv66t54r7s5oorwlhf2bwdxsshrjsmwgkdupcdhi2bqasa",
	}

	var qds []QuorumData
	for _, quorum := range faucetQuorumList {
		peerID, did, _ := util.ParseAddress(quorum)
		c.w.AddDIDPeerMap(did, peerID)
		qd := QuorumData{
			Type:    2,
			Address: did,
		}
		qds = append(qds, qd)
	}
	if err := c.RemoveAllQuorum(); err != nil {
		c.log.Error(fmt.Sprintf("AddDefaultTestnetQuorums: failed remove all existing quorums from quorummanger table, err: %v", err))
		return
	}

	if err := c.qm.AddQuorum(qds); err != nil {
		c.log.Error(fmt.Sprintf("AddDefaultTestnetQuorums: failed to add default quorums to the quorum manager table, err: %v", err))
		return
	}

	defaultQuorumFile := "default_quorums.json"

	currentDir, err := os.Getwd()
	if err != nil {
		c.log.Error(fmt.Sprintf("AddDefaultTestnetQuorums: unable to fetch current dir, err: %v", err))
		return
	}
	completeDefaultQuorumFilePath := path.Join(currentDir, defaultQuorumFile)

	c.log.Info("Default quorums have been added successfully and their info is save under %v", completeDefaultQuorumFilePath)
}
