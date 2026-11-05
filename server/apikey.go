package server

import (
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	model "github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// APIKeyFileName is the node-local file holding the API key; it is never served over HTTP.
const APIKeyFileName = ".api_key"

// APIKeyRecord is the on-disk API key; a zero ExpiresAt means unlimited validity.
type APIKeyRecord struct {
	Key       string    `json:"key"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Revoked   bool      `json:"revoked"`
}

// APIKeyPath returns the API key file path for a node directory.
func APIKeyPath(nodeDir string) string {
	return filepath.Join(nodeDir, APIKeyFileName)
}

// Expired reports whether the key has a validity limit that has passed.
func (r *APIKeyRecord) Expired(now time.Time) bool {
	return !r.ExpiresAt.IsZero() && now.After(r.ExpiresAt)
}

// Status returns "revoked", "expired" or "active".
func (r *APIKeyRecord) Status(now time.Time) string {
	if r.Revoked {
		return "revoked"
	}
	if r.Expired(now) {
		return "expired"
	}
	return "active"
}

// LoadAPIKey reads and strictly validates the API key file.
func LoadAPIKey(path string) (*APIKeyRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rec APIKeyRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, fmt.Errorf("malformed API key file %s: %v", path, err)
	}
	if rec.Revoked {
		return &rec, nil
	}
	if _, err := hex.DecodeString(rec.Key); err != nil || len(rec.Key) != 64 {
		return nil, fmt.Errorf("malformed API key file %s: key must be 64 hex characters", path)
	}
	return &rec, nil
}

// GenerateAPIKey writes a fresh key, replacing any existing one; validity 0 means unlimited.
func GenerateAPIKey(path string, validity time.Duration) (*APIKeyRecord, error) {
	if validity < 0 {
		return nil, fmt.Errorf("API key validity must not be negative")
	}
	key := util.GetRandString()
	if key == "" {
		return nil, fmt.Errorf("failed to generate random API key")
	}
	now := time.Now().UTC()
	rec := &APIKeyRecord{Key: key, CreatedAt: now}
	if validity > 0 {
		rec.ExpiresAt = now.Add(validity)
	}
	if err := writeAPIKey(path, rec); err != nil {
		return nil, err
	}
	return rec, nil
}

// RevokeAPIKey marks the key revoked and erases it, disabling protected APIs until regenerated.
func RevokeAPIKey(path string) error {
	rec, err := LoadAPIKey(path)
	if err != nil {
		return err
	}
	rec.Revoked = true
	rec.Key = ""
	return writeAPIKey(path, rec)
}

// writeAPIKey writes atomically (0600 temp file then rename) so the node never reads a partial file.
func writeAPIKey(path string, rec *APIKeyRecord) error {
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), APIKeyFileName+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// apiKeyStore caches the key file and reloads it when the file is replaced, so revoke/regenerate apply live.
type apiKeyStore struct {
	path string
	mu   sync.Mutex
	rec  *APIKeyRecord
	info os.FileInfo
}

// current returns the active record, or nil when the file is missing, malformed or unreadable.
func (st *apiKeyStore) current() *APIKeyRecord {
	st.mu.Lock()
	defer st.mu.Unlock()
	info, err := os.Stat(st.path)
	if err != nil {
		st.rec, st.info = nil, nil
		return nil
	}
	// Atomic rename gives a new inode, so SameFile catches a replace even within one mtime tick.
	if st.rec == nil || !os.SameFile(info, st.info) || !info.ModTime().Equal(st.info.ModTime()) {
		rec, err := LoadAPIKey(st.path)
		if err != nil {
			st.rec, st.info = nil, nil
			return nil
		}
		st.rec, st.info = rec, info
	}
	return st.rec
}

// requireAPIKey rejects the request unless it carries the node's active API key in X-API-Key.
func (s *Server) requireAPIKey(hf ensweb.HandlerFunc) ensweb.HandlerFunc {
	return func(req *ensweb.Request) *ensweb.Result {
		reject := func(msg string) *ensweb.Result {
			s.log.Error("API key check failed", "path", req.Path, "reason", msg)
			return s.RenderJSON(req, &model.BasicResponse{Status: false, Message: msg}, http.StatusUnauthorized)
		}
		rec := s.apiKeys.current()
		if rec == nil {
			return reject("API key not configured on this node")
		}
		if rec.Revoked {
			return reject("API key revoked, run regenerateapikey on the node")
		}
		if rec.Expired(time.Now()) {
			return reject("API key expired, run regenerateapikey on the node")
		}
		given := s.GetReqHeader(req, ensweb.APIKeyHeader)
		if subtle.ConstantTimeCompare([]byte(given), []byte(rec.Key)) != 1 {
			return reject("invalid or missing API key")
		}
		return hf(req)
	}
}
