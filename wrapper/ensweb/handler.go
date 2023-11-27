package ensweb

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"github.com/rubixchain/rubixgoplatform/wrapper/helper/jsonutil"
	"github.com/rubixchain/rubixgoplatform/wrapper/wraperr"
	"io"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// bufferedReader can be used to replace a request body with a buffered
// version. The Close method invokes the original Closer.
type bufferedReader struct {
	*bufio.Reader
	rOrig io.ReadCloser
}

func newBufferedReader(r io.ReadCloser) *bufferedReader {
	return &bufferedReader{
		Reader: bufio.NewReader(r),
		rOrig:  r,
	}
}

func (b *bufferedReader) Close() error {
	return b.rOrig.Close()
}

func parseQuery(values url.Values) map[string]interface{} {
	data := map[string]interface{}{}
	for k, v := range values {
		// Skip the help key as this is a reserved parameter
		if k == "help" {
			continue
		}

		switch {
		case len(v) == 0:
		case len(v) == 1:
			data[k] = v[0]
		default:
			data[k] = v
		}
	}

	if len(data) > 0 {
		return data
	}
	return nil
}

// isForm tries to determine whether the request should be
// processed as a form or as JSON.
//
// Virtually all existing use cases have assumed processing as JSON,
// and there has not been a Content-Type requirement in the API. In order to
// maintain backwards compatibility, this will err on the side of JSON.
// The request will be considered a form only if:
//
//  1. The content type is "application/x-www-form-urlencoded"
//  2. The start of the request doesn't look like JSON. For this test we
//     we expect the body to begin with { or [, ignoring leading whitespace.
func isForm(head []byte, contentType string) bool {
	contentType, _, err := mime.ParseMediaType(contentType)

	if err != nil || contentType != "application/x-www-form-urlencoded" {
		return false
	}

	// Look for the start of JSON or not-JSON, skipping any insignificant
	// whitespace (per https://tools.ietf.org/html/rfc7159#section-2).
	for _, c := range head {
		switch c {
		case ' ', '\t', '\n', '\r':
			continue
		case '[', '{': // JSON
			return false
		default: // not JSON
			return true
		}
	}

	return true
}

func parseJSONRequest(secondary bool, r *http.Request, w http.ResponseWriter, out interface{}) (io.ReadCloser, error) {
	reader := r.Body
	ctx := r.Context()
	maxRequestSize := ctx.Value("max_request_size")
	if maxRequestSize != nil {
		max, ok := maxRequestSize.(int64)
		if !ok {
			return nil, errors.New("could not parse max_request_size from request context")
		}
		if max > 0 {
			reader = http.MaxBytesReader(w, r.Body, max)
		}
	}

	var origBody io.ReadWriter
	if secondary {
		// Since we're checking PerfStandby here we key on origBody being nil
		// or not later, so we need to always allocate so it's non-nil
		origBody = new(bytes.Buffer)
		reader = ioutil.NopCloser(io.TeeReader(reader, origBody))
	}

	err := jsonutil.DecodeJSONFromReader(reader, out)
	if err != nil && err != io.EOF {
		return nil, wraperr.Wrapf(err, "failed to parse JSON input")
	}
	if origBody != nil {
		return ioutil.NopCloser(origBody), err
	} else {
		reader.Close()
	}
	return nil, err
}

// parseFormRequest parses values from a form POST.
//
// A nil map will be returned if the format is empty or invalid.
func parseFormRequest(r *http.Request) (map[string]interface{}, error) {
	maxRequestSize := r.Context().Value("max_request_size")
	if maxRequestSize != nil {
		max, ok := maxRequestSize.(int64)
		if !ok {
			return nil, errors.New("could not parse max_request_size from request context")
		}
		if max > 0 {
			r.Body = ioutil.NopCloser(io.LimitReader(r.Body, max))
		}
	}
	if err := r.ParseForm(); err != nil {
		return nil, err
	}

	var data map[string]interface{}

	if len(r.PostForm) != 0 {
		data = make(map[string]interface{}, len(r.PostForm))
		for k, v := range r.PostForm {
			switch len(v) {
			case 0:
			case 1:
				data[k] = v[0]
			default:
				// Almost anywhere taking in a string list can take in comma
				// separated values, and really this is super niche anyways
				data[k] = strings.Join(v, ",")
			}
		}
	}

	return data, nil
}

func basicHandleFunc(s *Server, hf HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		req := basicRequestFunc(s, w, r)

		res := hf(req)
		if res != nil && s.auditLog != nil {
			timeDuration := time.Now().Nanosecond() - req.TimeIn.Nanosecond()
			userAgent := r.Header.Get("User-Agent")
			if res.Done {
				s.auditLog.Info("HTTP request processed", "Path", req.Path, "IP Address", req.Connection.RemoteAddr, "Status", res.Status, "Duration", timeDuration, "User-Agent", userAgent)
			} else {
				s.auditLog.Error("HTTP request failed", "Path", req.Path, "IP Address", req.Connection.RemoteAddr, "Duration", timeDuration, "User-Agent", userAgent)
			}
		}

	})
}

func indexRoute(s *Server, dirPath string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fs := http.FileServer(http.Dir(dirPath))

		// If the requested file exists then return if; otherwise return index.html (fileserver default page)
		if r.URL.Path != "/" {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, s.prefixPath)
			fullPath := dirPath + r.URL.Path
			_, err := os.Stat(fullPath)
			if err != nil {
				if !os.IsNotExist(err) {
					panic(err)
				}
				fmt.Printf("Not Found : %s\n", r.URL.Path)
				// Requested file does not exist so we return the default (resolves to index.html)
				r.URL.Path = "/"
			}
		}

		if s.debugMode {
			enableCors(&w)
		}

		fs.ServeHTTP(w, r)
	})
}

func (s *Server) AddExtension(fileExtension string, fileType string) {
	mime.AddExtensionType(fileExtension, fileType)
}

func (s *Server) IsFORM(req *Request) (bool, error) {
	bufferedBody := newBufferedReader(req.r.Body)
	req.r.Body = bufferedBody
	head, err := bufferedBody.Peek(512)
	if err != nil && err != bufio.ErrBufferFull && err != io.EOF {
		return false, fmt.Errorf("error reading data")
	}
	if isForm(head, req.r.Header.Get("Content-Type")) {
		return true, nil
	}
	return false, nil
}

func (s *Server) ParseJSON(req *Request, model interface{}) error {
	_, err := parseJSONRequest(false, req.r, req.w, model)
	return err
}

func (s *Server) ParseFORM(req *Request) (map[string]interface{}, error) {
	formData, err := parseFormRequest(req.r)
	if err != nil {
		return nil, fmt.Errorf("error parsing form data")
	}
	return formData, nil
}

func (s *Server) GetQuerry(req *Request, key string) string {
	return req.r.URL.Query().Get(key)
}

func (s *Server) ParseMultiPartForm(req *Request, dirPath string) ([]string, map[string][]string, error) {
	mediatype, _, err := mime.ParseMediaType(req.r.Header.Get("Content-Type"))
	if err != nil {
		return nil, nil, err
	}
	if mediatype != "multipart/form-data" {
		return nil, nil, fmt.Errorf("invalid content type")
	}
	defer req.r.Body.Close()

	req.r.ParseMultipartForm(52428800)
	paramFiles := make([]string, 0)
	paramTexts := make(map[string][]string)
	for k, v := range req.r.MultipartForm.Value {
		paramTexts[k] = append(paramTexts[k], v...)
	}

	for k, _ := range req.r.MultipartForm.File {
		file, fileHeader, err := req.r.FormFile(k)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid form file")
		}
		localFileName := dirPath + fileHeader.Filename
		out, err := os.OpenFile(localFileName, os.O_CREATE|os.O_RDWR, 0777)
		if err != nil {
			file.Close()
			return nil, nil, fmt.Errorf("faile to open file")
		}
		_, err = io.Copy(out, file)
		if err != nil {
			file.Close()
			out.Close()
			return nil, nil, fmt.Errorf("faile to copy file")
		}
		out.Close()
		file.Close()
		paramFiles = append(paramFiles, localFileName)
	}

	return paramFiles, paramTexts, nil
}

func (s *Server) Redirect(req *Request, url string) *Result {
	r := req.r
	w := req.w
	http.Redirect(w, r, url, http.StatusSeeOther)
	return &Result{
		Status: http.StatusSeeOther,
		Done:   true,
	}
}

func (s *Server) ServerStatic(req *Request) *Result {
	r := req.r
	w := req.w

	fs := http.FileServer(http.Dir(s.publicPath))

	// If the requested file exists then return if; otherwise return index.html (fileserver default page)
	if r.URL.Path != "/" {
		r.URL.Path = strings.TrimPrefix(r.URL.Path, s.prefixPath)
		fullPath := s.publicPath + r.URL.Path
		_, err := os.Stat(fullPath)
		if err != nil {
			if !os.IsNotExist(err) {
				panic(err)
			}
			fmt.Println("Not Found : ", r.URL.Path)
			// Requested file does not exist so we return the default (resolves to index.html)
			r.URL.Path = "/"
		}
	}
	fs.ServeHTTP(w, r)
	res := &Result{
		Status: http.StatusOK,
		Done:   true,
	}
	return res
}

func (s *Server) ParseMultiPartFormFile(req *Request, paramName string) (*os.File, *multipart.FileHeader, error) {
	mediatype, _, err := mime.ParseMediaType(req.r.Header.Get("Content-Type"))
	if err != nil {
		return nil, nil, err
	}
	if mediatype != "multipart/form-data" {
		return nil, nil, fmt.Errorf("invalid content type")
	}

	file, fileHeader, err := req.r.FormFile(paramName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to retrieve file")
	}
	defer file.Close()

	// Create a new *os.File and copy the contents of the multipart.File to it
	destFile, err := os.CreateTemp("", "file")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create temp file")
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, file)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to copy file contents")
	}

	// Seek back to the beginning of the file
	_, err = destFile.Seek(0, io.SeekStart)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to seek file")
	}

	return destFile, fileHeader, nil
}

// func (s *Server) ParseMultiPartForm(req *Request, dirPath string) ([]string, map[string]string, error) {
// 	mediatype, params, err := mime.ParseMediaType(req.r.Header.Get("Content-Type"))
// 	if err != nil {
// 		return nil, nil, err
// 	}
// 	if mediatype != "multipart/form-data" {
// 		return nil, nil, fmt.Errorf("invalid content type")
// 	}
// 	defer req.r.Body.Close()
// 	mr := multipart.NewReader(req.r.Body, params["boundary"])

// 	paramFiles := make([]string, 0)
// 	paramTexts := make(map[string]string)
// 	for {
// 		part, err := mr.NextPart()
// 		if err != nil {
// 			if err != io.EOF { //io.EOF error means reading is complete
// 				return paramFiles, paramTexts, fmt.Errorf(" error reading multipart request: %+v", err)
// 			}
// 			break
// 		}
// 		if part.FileName() != "" {
// 			chunk := make([]byte, 4096)
// 			f, err := os.OpenFile(dirPath+part.FileName(), os.O_WRONLY|os.O_CREATE, 0666)
// 			if err != nil {
// 				return paramFiles, paramTexts, fmt.Errorf("error in creating file %+v", err)
// 			}
// 			for {
// 				n, err := part.Read(chunk)
// 				if err != nil {
// 					if err != io.EOF {
// 						return paramFiles, paramTexts, fmt.Errorf(" error reading multipart file %+v", err)
// 					}
// 					if n > 0 {
// 						f.Write(chunk[:n])
// 					}
// 					break
// 				} else {
// 					if n > 0 {
// 						f.Write(chunk[:n])
// 					}
// 				}
// 			}
// 			f.Close()
// 			if err != nil {
// 				return paramFiles, paramTexts, fmt.Errorf("error reading file param %+v", err)
// 			}
// 			paramFiles = append(paramFiles, dirPath+part.FileName())
// 		} else {
// 			name := part.FormName()
// 			buf := new(bytes.Buffer)
// 			buf.ReadFrom(part)
// 			paramTexts[name] = buf.String()
// 		}
// 	}
// 	return paramFiles, paramTexts, nil
// }
