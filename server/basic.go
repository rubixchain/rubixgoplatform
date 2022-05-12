package server

import (
	"fmt"
	"net/http"

	"github.com/EnsurityTechnologies/ensweb"
)

// BasicResponse will send basic mode response
func (s *Server) BasicResponse(req *ensweb.Request, status bool, msg string, result interface{}) *ensweb.Result {
	resp := Repsonse{
		Status:  status,
		Message: msg,
		Result:  result,
	}
	return s.RenderJSON(req, &resp, http.StatusOK)
}

// APIStart will setup the core
func (s *Server) APIStart(req *ensweb.Request) *ensweb.Result {
	status, msg := s.c.Start()
	return s.BasicResponse(req, status, msg, nil)
}

// APIPing will ping to given peer
func (s *Server) APIPing(req *ensweb.Request) *ensweb.Result {
	peerdID := s.GetQuerry(req, "peerID")
	str, err := s.c.PingPeer(peerdID)
	if err != nil {
		s.log.Error("ping failed", "err", err)
		return s.BasicResponse(req, false, str, nil)
	}
	return s.BasicResponse(req, true, str, nil)
}

// APICreateDID will create new DID
func (s *Server) APICreateDID(req *ensweb.Request) *ensweb.Result {

	folderName, err := s.c.CreateTempFolder()
	if err != nil {
		s.log.Error("failed to create folder")
		return s.BasicResponse(req, false, "failed to create folder", nil)
	}

	fileNames, fieldNames, err := s.ParseMultiPartForm(req, folderName+"/")

	fmt.Printf("Field : %v, Files : %v\n", fileNames, fieldNames)

	if err != nil {
		s.log.Error("failed to parse request", "err", err)
		return s.BasicResponse(req, false, "failed to create DID", nil)
	}
	fields := fieldNames[SecretField]
	if len(fileNames) == 0 || len(fields) == 0 {
		s.log.Error("missing secret or image file")
		return s.BasicResponse(req, false, "missing secret or image file", nil)
	}
	did, err := s.c.CreateDID(fieldNames[SecretField][0], fileNames[0])
	if err != nil {
		s.log.Error("failed to create did", "err", err)
		return s.BasicResponse(req, false, err.Error(), nil)
	}
	didResp := DIDResponse{
		DID: did,
	}
	return s.BasicResponse(req, true, "DID Created successfully", &didResp)
}
