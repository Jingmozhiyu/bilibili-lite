package server

import (
	"io"
	"mime"
	"net/http"

	"github.com/go-kratos/kratos/v3/errors"
	kratosHTTP "github.com/go-kratos/kratos/v3/transport/http"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// Kratos v3's plain JSON codec uses encoding/json. Mall explicitly exposes
// protobuf JSON (RFC3339 timestamps, camelCase names, string int64 values).
func decodeJSON(r *http.Request, v any) error {
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		return errors.BadRequest("CODEC", "Content-Type must be application/json")
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 64*1024+1))
	if err != nil {
		return errors.BadRequest("CODEC", "cannot read body")
	}
	if len(data) > 64*1024 {
		return errors.BadRequest("CODEC", "body exceeds 64 KiB")
	}
	if len(data) == 0 {
		return nil
	}
	msg, ok := v.(proto.Message)
	if !ok {
		return errors.BadRequest("CODEC", "expected protobuf request")
	}
	if err = protojson.Unmarshal(data, msg); err != nil {
		return errors.BadRequest("CODEC", "invalid protobuf JSON")
	}
	return nil
}

func encodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	msg, ok := v.(proto.Message)
	if !ok {
		return kratosHTTP.DefaultResponseEncoder(w, r, v)
	}
	data, err := (protojson.MarshalOptions{EmitUnpopulated: true}).Marshal(msg)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(data)
	return err
}
