package command

func (cmd *Command) createFT() {
	if cmd.did == "" {
		cmd.log.Error("Failed to create FT, DID is required to create FT")
		return
	}

	br, err := cmd.c.CreateFT(cmd.did, cmd.ftName, cmd.ftCount, cmd.rbtAmount)

	if err != nil {
		cmd.log.Error("Failed to create FT", "err", err)
		return
	}
	if !br.Status {
		cmd.log.Error("Failed to create FT", "msg", br.Message)
		return
	}
	msg, status := cmd.SignatureResponse(br)
	if !status {
		cmd.log.Error("Failed to create FT, " + msg)
		return
	}
	cmd.log.Info("FT created successfully")
}
