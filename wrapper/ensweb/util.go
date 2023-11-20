package ensweb

import (
	"net/http"

	"github.com/EnsurityTechnologies/helper/jsonutil"
)

func JSONDecodeErr(resp *http.Response) (*ErrMessage, error) {
	var model ErrMessage
	err := jsonutil.DecodeJSONFromReader(resp.Body, &model)
	if err != nil {
		return nil, err
	}
	return &model, nil
}
