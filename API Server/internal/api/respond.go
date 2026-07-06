package api

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"

	"smsgateway/apiserver/internal/pb"
)

// writeJSON writes v as a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON: %v", err)
	}
}

// errorBody is the standard error envelope.
type errorBody struct {
	Error string `json:"error"`
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorBody{Error: msg})
}

// writeUpstreamError maps a PocketBase error onto an appropriate HTTP status.
func writeUpstreamError(w http.ResponseWriter, err error) {
	var apiErr *pb.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Status {
		case http.StatusNotFound:
			writeError(w, http.StatusNotFound, "not found")
		case http.StatusBadRequest:
			writeError(w, http.StatusBadRequest, apiErr.Message)
		case http.StatusForbidden:
			writeError(w, http.StatusForbidden, "forbidden")
		default:
			log.Printf("upstream error: %v (%s)", apiErr, apiErr.Body)
			writeError(w, http.StatusBadGateway, "upstream error")
		}
		return
	}
	log.Printf("internal error: %v", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

// decodeJSON reads and decodes a JSON request body into v.
func decodeJSON(r *http.Request, v any) error {
	defer io.Copy(io.Discard, r.Body)
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	return nil
}
