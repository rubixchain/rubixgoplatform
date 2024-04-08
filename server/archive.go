package server

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

func (s *Server) APIRecoverArchive(request *ensweb.Request) *ensweb.Result {
	var recoverArchiveReq *model.RecoverArchiveReq
	err := s.ParseJSON(request, &recoverArchiveReq)
	if err != nil {
		return s.BasicResponse(request, false, "Failed to Recover Archive", nil)
	}
	s.c.RecoverArchive(recoverArchiveReq.Did, recoverArchiveReq.ArchivePath)
	return s.BasicResponse(request, true, "Archive Recovered Successfully", nil)
}

func (s *Server) APIArchive(request *ensweb.Request) *ensweb.Result {
	var archiveReq *model.RecoverArchiveReq
	err := s.ParseJSON(request, &archiveReq)
	if err != nil {
		return s.BasicResponse(request, false, "Failed to Archive DID", nil)
	}
	s.c.Archive(archiveReq.Did, archiveReq.ArchivePath)
	return s.BasicResponse(request, true, "Archive Recovered Successfully", nil)
}
