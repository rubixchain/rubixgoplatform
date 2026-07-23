package recovery

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// Safety limits for the recover-from-fullnode endpoint.
//
// Pagination is a two-phase, size-limited cursor: token rows keyed on
// (token_id, position), then transactions keyed on tx id. Each response is
// gzipped with an explicit Content-Length so the client reads an exact number of
// bytes and avoids the p2p-forward truncation seen on chunked responses. With
// the connection kept alive, a page is limited only by the server WriteTimeout
// (~60s) and memory, so the size limit is set high to keep the round-trip count
// low.
const (
	// recoverMaxCompressedBytes is the page size limit, checked on the gzipped
	// body. After each row is added the handler marshals and gzips the candidate
	// response and stops if it would exceed this. This packs many small entries
	// per page and still sends an oversize single entry on its own (the first
	// entry always ships). 1 MB compressed sits well within the WriteTimeout and
	// memory; lower it only if a relay path shows truncation.
	recoverMaxCompressedBytes = 1 * 1024 * 1024

	// recoverBatchSize caps the DB rows fetched per page build so one request
	// can't pull an unbounded amount into memory. The size limit above is the
	// real bound; this just caps a single fetch.
	recoverBatchSize = 2000

	// recoverMaxRequestBodyBytes caps the incoming request body, which is small
	// (did plus cursor).
	recoverMaxRequestBodyBytes = 64 * 1024
)

// renderGzipFixedLengthJSON writes a gzipped JSON response with an explicit
// Content-Length. Setting Content-Length with Content-Encoding: gzip keeps Go's
// HTTP server on identity transfer (not chunked), which avoids the p2p-forward
// truncation seen on larger chunked responses. The client's http.Transport
// decompresses gzip automatically, so the caller sees a normal body. On gzip
// failure it falls back to an uncompressed identity-encoded response.
func renderGzipFixedLengthJSON(req *ensweb.Request, body interface{}, status int) *ensweb.Result {
	raw, err := json.Marshal(body)
	if err != nil {
		w := req.GetHTTPWritter()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"status":false,"message":"marshal failed"}`))
		return &ensweb.Result{Status: http.StatusInternalServerError, Done: true}
	}

	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	if _, werr := gz.Write(raw); werr != nil {
		return writeIdentityJSON(req, raw, status)
	}
	if cerr := gz.Close(); cerr != nil {
		return writeIdentityJSON(req, raw, status)
	}

	w := req.GetHTTPWritter()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Content-Length", strconv.Itoa(compressed.Len()))
	w.WriteHeader(status)
	_, _ = w.Write(compressed.Bytes())
	return &ensweb.Result{Status: status, Done: true}
}

// writeIdentityJSON is the gzip-failure fallback: a fixed-length,
// identity-encoded JSON response.
func writeIdentityJSON(req *ensweb.Request, raw []byte, status int) *ensweb.Result {
	w := req.GetHTTPWritter()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(raw)))
	w.WriteHeader(status)
	_, _ = w.Write(raw)
	return &ensweb.Result{Status: status, Done: true}
}

// compressedSize reports the gzipped length of raw. The handler uses it while
// filling a page to check whether one more row would exceed the size limit.
func compressedSize(raw []byte) (int, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(raw); err != nil {
		return 0, err
	}
	if err := gz.Close(); err != nil {
		return 0, err
	}
	return buf.Len(), nil
}

// pageFits marshals and gzips the candidate response and reports its compressed
// size and whether it stays within the page size limit. The first entry on a
// page always ships (a later retry covers the rare oversize single entry), so
// callers pass includedSoFar and the limit is applied only once one entry is
// committed. A marshal or gzip failure counts as "does not fit", again only
// after the first entry.
func pageFits(body *models.BasicResponse, includedSoFar int) (compressed int, fits bool) {
	raw, mErr := json.Marshal(body)
	if mErr != nil {
		return 0, includedSoFar == 0
	}
	cz, czErr := compressedSize(raw)
	if czErr != nil {
		return 0, includedSoFar == 0
	}
	if includedSoFar > 0 && cz > recoverMaxCompressedBytes {
		return cz, false
	}
	return cz, true
}
