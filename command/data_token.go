package command

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path"

	"github.com/rubixchain/rubixgoplatform/client"
	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/util"
)

func (cmd *Command) createDataToken() {
	dt := client.DataTokenReq{
		DID:      cmd.did,
		UserID:   cmd.userID,
		UserInfo: cmd.userInfo,
	}

	if cmd.fileMode {
		dt.Files = make([]string, 0)
		dt.Files = append(dt.Files, cmd.file)
	} else {
		fd, err := ioutil.ReadFile(cmd.file)
		if err != nil {
			cmd.log.Error("Failed to read file", "err", err)
			return
		}
		hb := util.CalculateHash(fd, "SHA3-256")
		fi := make(map[string]map[string]string)
		fn := path.Base(cmd.file)
		info := make(map[string]string)
		info[core.DTFileHashField] = util.HexToStr(hb)
		fi[fn] = info
		jb, err := json.Marshal(fi)
		if err != nil {
			cmd.log.Error("Failed to marshal json input", "err", err)
			return
		}
		dt.FileInfo = string(jb)
	}
	br, err := cmd.c.CreateDataToken(&dt)
	if err != nil {
		cmd.log.Error("Failed to create data token", "err", err)
		return
	}
	if !br.Status {
		cmd.log.Error("Failed to create data token", "msg", br.Message)
		return
	}
	msg, status := cmd.SignatureResponse(br)
	if !status {
		cmd.log.Error("Failed to create data token, " + msg)
		return
	}
	cmd.log.Info(fmt.Sprintf("Data Token : %s", msg))
	cmd.log.Info("Data token created successfully")
}

func (cmd *Command) commitDataToken() {
	br, err := cmd.c.CommitDataToken(cmd.did, cmd.batchID)
	if err != nil {
		cmd.log.Error("Failed to commit data token", "err", err)
		return
	}

	if !br.Status {
		cmd.log.Error("Failed to commit data token", "msg", br.Message)
		return
	}

	msg, status := cmd.SignatureResponse(br)

	if !status {
		cmd.log.Error("Failed to commit data token, " + msg)
		return
	}
	cmd.log.Info("Data tokens committed successfully")
}
