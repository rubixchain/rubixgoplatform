package model

type RecoverArchiveReq struct {
	Did         string `json:"did"`
	ArchivePath string `json:"archivepath"`
}
