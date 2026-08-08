package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// DecodeJSON decodes exactly one JSON value and rejects unknown fields.
func DecodeJSON(request *http.Request, destination any) error {
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode JSON body: %w", err)
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode JSON body: multiple JSON values")
		}
		return fmt.Errorf("decode JSON body trailer: %w", err)
	}
	return nil
}
