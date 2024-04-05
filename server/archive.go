package server

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// type RecoverArchiveReq struct {
// 	Did string `json:"did"`
// }

func (s *Server) APIRecoverArchive(request *ensweb.Request) *ensweb.Result {
	var recoverArchiveReq *model.RecoverArchiveReq
	err := s.ParseJSON(request, &recoverArchiveReq)
	if err != nil {
		return s.BasicResponse(request, false, "Failed to Recover Archive", nil)
	}
	s.c.RecoverArchive(recoverArchiveReq.Did, recoverArchiveReq.ArchivePath)
	return s.BasicResponse(request, true, "Archive Recovered Successfully", nil)
}
