package ensweb

import (
	"bytes"
	"crypto/tls"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/wrapper/uuid"
)

const (
	APIKeyHeader string = "X-API-Key"
)

// Operation is an enum that is used to specify the type
// of request being made
type Operation string

type CookiesType map[interface{}]interface{}

type Request struct {
	ID          string
	Method      string
	Path        string
	TimeIn      time.Time
	ClientToken ClientToken
	Connection  *Connection
	Data        map[string]interface{} `json:"data" structs:"data" mapstructure:"data"`
	Model       interface{}
	Headers     http.Header
	TenantID    uuid.UUID
	r           *http.Request
	w           http.ResponseWriter `json:"-" sentinel:""`
}

type ClientToken struct {
	Token          string
	BearerToken    bool
	APIKeyVerified bool
	Verified       bool
	Model          interface{}
}

// Connection represents the connection information for a request.
type Connection struct {
	// RemoteAddr is the network address that sent the request.
	RemoteAddr string `json:"remote_addr"`

	// ConnState is the TLS connection state if applicable.
	ConnState *tls.ConnectionState `sentinel:""`
}

// ipRange - a structure that holds the start and end of a range of ip addresses
type ipRange struct {
	start net.IP
	end   net.IP
}

// inRange - check to see if a given ip address is within a range given
func inRange(r ipRange, ipAddress net.IP) bool {
	// strcmp type byte comparison
	if bytes.Compare(ipAddress, r.start) >= 0 && bytes.Compare(ipAddress, r.end) < 0 {
		return true
	}
	return false
}

var privateRanges = []ipRange{
	ipRange{
		start: net.ParseIP("10.0.0.0"),
		end:   net.ParseIP("10.255.255.255"),
	},
	ipRange{
		start: net.ParseIP("100.64.0.0"),
		end:   net.ParseIP("100.127.255.255"),
	},
	ipRange{
		start: net.ParseIP("172.16.0.0"),
		end:   net.ParseIP("172.31.255.255"),
	},
	ipRange{
		start: net.ParseIP("192.0.0.0"),
		end:   net.ParseIP("192.0.0.255"),
	},
	ipRange{
		start: net.ParseIP("192.168.0.0"),
		end:   net.ParseIP("192.168.255.255"),
	},
	ipRange{
		start: net.ParseIP("198.18.0.0"),
		end:   net.ParseIP("198.19.255.255"),
	},
}

// isPrivateSubnet - check to see if this ip is in a private subnet
func isPrivateSubnet(ipAddress net.IP) bool {
	// my use case is only concerned with ipv4 atm
	if ipCheck := ipAddress.To4(); ipCheck != nil {
		// iterate over all our ranges
		for _, r := range privateRanges {
			// check if this ip is in a private range
			if inRange(r, ipAddress) {
				return true
			}
		}
	}
	return false
}

func getIPAdress(r *http.Request) string {
	privIP := ""
	for _, h := range []string{"X-Forwarded-For", "X-Real-Ip"} {
		addresses := strings.Split(r.Header.Get(h), ",")
		// march from right to left until we get a public address
		// that will be the address right before our proxy.
		for i := len(addresses) - 1; i >= 0; i-- {
			//ip := strings.TrimSpace(addresses[i])
			ip, _, err := net.SplitHostPort(addresses[i])
			if err != nil {
				continue
			}
			// header can contain spaces too, strip those out.
			realIP := net.ParseIP(ip)
			if !realIP.IsGlobalUnicast() {
				continue
			} else if isPrivateSubnet(realIP) {
				// bad address, go to next
				if privIP == "" {
					privIP = ip
				}
				continue
			}
			return ip
		}
	}
	return privIP
}

// getConnection is used to format the connection information
func getConnection(r *http.Request) (connection *Connection) {
	var remoteAddr string
	remoteAddr = getIPAdress(r)
	var err error
	if remoteAddr == "" {
		remoteAddr, _, err = net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			remoteAddr = ""
		}
	}

	connection = &Connection{
		RemoteAddr: remoteAddr,
		ConnState:  r.TLS,
	}
	return
}

func (req *Request) GetHTTPRequest() *http.Request {
	return req.r
}

func (req *Request) GetHTTPWritter() http.ResponseWriter {
	return req.w
}

func (s *Server) getTenantID(r *http.Request) uuid.UUID {
	if s.tcb == nil {
		return s.defaultTenantID
	}
	url := r.Host
	url = strings.TrimPrefix(url, "https://")
	return s.tcb(url)
}

func basicRequestFunc(s *Server, w http.ResponseWriter, r *http.Request) *Request {

	path := r.URL.Path

	requestId := uuid.New().String()

	req := &Request{
		ID:          requestId,
		Method:      r.Method,
		Path:        path,
		TimeIn:      time.Now(),
		ClientToken: getTokenFromReq(s, r),
		Connection:  getConnection(r),
		Headers:     r.Header,
		TenantID:    s.getTenantID(r),
		r:           r,
		w:           w,
	}

	return req

}

// getTokenFromReq parse headers of the incoming request to extract token if
// present it accepts Authorization Bearer (RFC6750) and configured header.
// Returns true if the token was sourced from a Bearer header.
func getTokenFromReq(s *Server, r *http.Request) ClientToken {
	if s.serverCfg != nil && s.serverCfg.AuthHeaderName != "" {
		if token := r.Header.Get(s.serverCfg.AuthHeaderName); token != "" {
			return ClientToken{Token: token, BearerToken: false}
		}
	}
	if headers, ok := r.Header["Authorization"]; ok {
		// Reference for Authorization header format: https://tools.ietf.org/html/rfc7236#section-3

		// If string does not start by 'Bearer ', it is not one we would use,
		// but might be used by plugins
		for _, v := range headers {
			if !strings.HasPrefix(v, "Bearer ") {
				continue
			}
			return ClientToken{Token: strings.TrimSpace(v[7:]), BearerToken: true}
		}
	}
	return ClientToken{Token: "", BearerToken: false}
}

func (s *Server) GetReqHeader(req *Request, key string) string {
	return req.r.Header.Get(key)
}
