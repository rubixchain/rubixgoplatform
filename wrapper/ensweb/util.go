package ensweb

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/rubixchain/rubixgoplatform/wrapper/helper/jsonutil"
)

func JSONDecodeErr(resp *http.Response) (*ErrMessage, error) {
	var model ErrMessage
	err := jsonutil.DecodeJSONFromReader(resp.Body, &model)
	if err != nil {
		return nil, err
	}
	return &model, nil
}

// SubstitutePathParams the map and tries to find each key in the endpoint,
// and then substitute each param value in the endpoint
func SubstitutePathParams(endpoint string, params map[string]string) (string, error) {
	for key, value := range params {
		endpoint = strings.ReplaceAll(endpoint, "{"+key+"}", url.PathEscape(value))
	}

	// Check if any unsubstituted placeholders remain
	if strings.Contains(endpoint, "{") && strings.Contains(endpoint, "}") {
		return "", fmt.Errorf("unsubstituted placeholders found in endpoint: %s", endpoint)
	}

	return endpoint, nil
}
