package ensweb

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/wrapper/config"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// Client : Client struct
type Client struct {
	config         *config.Config
	log            logger.Logger
	address        string
	addr           *url.URL
	hc             *http.Client
	th             TokenHelper
	defaultTimeout time.Duration
	token          string
	cookies        []*http.Cookie
}

type ClientOptions = func(*Client) error

func SetClientDefaultTimeout(timeout time.Duration) ClientOptions {
	return func(c *Client) error {
		c.defaultTimeout = timeout
		return nil
	}
}

func SetClientTokenHelper(filename string) ClientOptions {
	return func(c *Client) error {
		th, err := NewInternalTokenHelper(filename)
		if err != nil {
			return err
		}
		c.th = th
		return nil
	}
}

// NewClient : Create new client handle
func NewClient(config *config.Config, log logger.Logger, options ...ClientOptions) (Client, error) {
	var address string
	var tr *http.Transport
	clog := log.Named("enswebclient")
	if config.Production == "true" {
		address = fmt.Sprintf("https://%s", net.JoinHostPort(config.ServerAddress, config.ServerPort))
		tr = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	} else {
		address = fmt.Sprintf("http://%s", net.JoinHostPort(config.ServerAddress, config.ServerPort))
		tr = &http.Transport{
			IdleConnTimeout: 30 * time.Second,
		}
	}

	hc := &http.Client{
		Transport: tr,
		Timeout:   DefaultTimeout,
	}

	addr, err := url.Parse(address)

	if err != nil {
		clog.Error("failed to parse server address", "err", err)
		return Client{}, err
	}

	tc := Client{
		config:  config,
		log:     clog,
		address: address,
		addr:    addr,
		hc:      hc,
	}

	for _, op := range options {
		err = op(&tc)
		if err != nil {
			clog.Error("failed in setting the option", "err", err)
			return Client{}, err
		}
	}
	return tc, nil
}

func (c *Client) JSONRequest(method string, requestPath string, model interface{}) (*http.Request, error) {
	var body *bytes.Buffer
	if model != nil {
		j, err := json.Marshal(model)
		if err != nil {
			return nil, err
		}
		body = bytes.NewBuffer(j)
	} else {
		body = bytes.NewBuffer(make([]byte, 0))
	}
	url := &url.URL{
		Scheme: c.addr.Scheme,
		Host:   c.addr.Host,
		User:   c.addr.User,
		Path:   path.Join(c.addr.Path, requestPath),
	}
	req, err := http.NewRequest(method, url.RequestURI(), body)
	req.Host = url.Host
	req.URL.User = url.User
	req.URL.Scheme = url.Scheme
	req.URL.Host = url.Host
	req.Header.Set("Content-Type", "application/json")
	return req, err
}

func (c *Client) MultiFormRequest(method string, requestPath string, field map[string]string, files map[string]string) (*http.Request, error) {
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	for k, v := range field {
		fw, err := w.CreateFormField(k)
		if err != nil {
			return nil, err
		}
		if _, err = io.Copy(fw, strings.NewReader(v)); err != nil {
			return nil, err
		}
	}
	for k, v := range files {
		fw, err := w.CreateFormFile(k, filepath.Base(v))
		if err != nil {
			return nil, err
		}
		f, err := os.Open(v)
		if err != nil {
			return nil, err
		}
		if _, err = io.Copy(fw, f); err != nil {
			return nil, err
		}
	}
	err := w.Close()
	if err != nil {
		return nil, err
	}

	url := &url.URL{
		Scheme: c.addr.Scheme,
		Host:   c.addr.Host,
		User:   c.addr.User,
		Path:   path.Join(c.addr.Path, requestPath),
	}
	req, err := http.NewRequest(method, url.RequestURI(), &b)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Host = url.Host
	req.URL.User = url.User
	req.URL.Scheme = url.Scheme
	req.URL.Host = url.Host
	return req, err
}

func (c *Client) SetAuthorization(req *http.Request, token string) {
	var bearer = "Bearer " + token
	req.Header.Set("Authorization", bearer)
}

func (c *Client) Do(req *http.Request, timeout ...time.Duration) (*http.Response, error) {
	if timeout != nil {
		c.hc.Timeout = timeout[0]
	} else {
		c.hc.Timeout = c.defaultTimeout
	}
	return c.hc.Do(req)
}

func (c *Client) SetCookies(cookies []*http.Cookie) {
	c.cookies = cookies
}

func (c *Client) GetCookies() []*http.Cookie {
	return c.cookies
}

func (c *Client) SetToken(token string) error {
	if c.th != nil {
		return c.th.Store(token)
	}
	c.token = token
	return nil
}

func (c *Client) GetToken() string {
	if c.th != nil {
		tk, err := c.th.Get()
		if err != nil {
			return "InvalidToken"
		} else {
			return tk
		}
	}
	return c.token
}

func (c *Client) ParseMutilform(resp *http.Response, dirPath string) ([]string, map[string]string, error) {
	mediatype, params, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		return nil, nil, err
	}
	if mediatype != "multipart/form-data" {
		return nil, nil, fmt.Errorf("invalid content type %s", mediatype)
	}
	defer resp.Body.Close()
	mr := multipart.NewReader(resp.Body, params["boundary"])

	paramFiles := make([]string, 0)
	paramTexts := make(map[string]string)
	for {
		part, err := mr.NextPart()
		if err != nil {
			if err != io.EOF { //io.EOF error means reading is complete
				return paramFiles, paramTexts, fmt.Errorf(" error reading multipart request: %+v", err)
			}
			break
		}
		if part.FileName() != "" {
			f, err := os.Create(dirPath + part.FileName())
			if err != nil {
				return paramFiles, paramTexts, fmt.Errorf("error in creating file %+v", err)
			}
			value, _ := ioutil.ReadAll(part)
			f.Write(value)
			f.Close()
			if err != nil {
				return paramFiles, paramTexts, fmt.Errorf("error reading file param %+v", err)
			}
			paramFiles = append(paramFiles, dirPath+part.FileName())
		} else {
			name := part.FormName()
			buf := new(bytes.Buffer)
			buf.ReadFrom(part)
			paramTexts[name] = buf.String()
		}
	}
	return paramFiles, paramTexts, nil
}
