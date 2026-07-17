package service

import (
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"bilibili-lite/internal/biz"
	"bilibili-lite/internal/conf"

	kratosHTTP "github.com/go-kratos/kratos/v3/transport/http"
)

// VideoUploadHTTPHandler streams one multipart MP4 directly into the media usecase.
type VideoUploadHTTPHandler struct {
	videoUsecase *biz.VideoUsecase
	userUsecase  *biz.UserUsecase
	maxBytes     int64
	idleTimeout  time.Duration
}

// idleDeadlineReader refreshes the HTTP read deadline whenever upload data is consumed.
type idleDeadlineReader struct {
	source     io.Reader
	controller *http.ResponseController
	timeout    time.Duration
}

// NewVideoUploadHTTPHandler creates the authenticated HTTP adapter for streaming MP4 uploads.
func NewVideoUploadHTTPHandler(videoUsecase *biz.VideoUsecase, userUsecase *biz.UserUsecase, dataConfig *conf.Data) *VideoUploadHTTPHandler {
	return &VideoUploadHTTPHandler{
		videoUsecase: videoUsecase,
		userUsecase:  userUsecase,
		maxBytes:     dataConfig.GetMedia().GetMaxUploadBytes(),
		idleTimeout:  dataConfig.GetMedia().GetUploadIdleTimeout().AsDuration(),
	}
}

// newIdleDeadlineReader decorates an upload stream with per-read HTTP idle deadlines.
func newIdleDeadlineReader(source io.Reader, writer http.ResponseWriter, timeout time.Duration) *idleDeadlineReader {
	return &idleDeadlineReader{
		source:     source,
		controller: http.NewResponseController(writer),
		timeout:    timeout,
	}
}

// ServeHTTP authenticates a multipart request and passes its MP4 part to the upload usecase as an io.Reader.
func (h *VideoUploadHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, err := h.userUsecase.AuthenticateAccess(parseBearerToken(r.Header.Get("Authorization")))
	if err != nil {
		kratosHTTP.DefaultErrorEncoder(w, r, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBytes+1024*1024)
	multipartReader, err := r.MultipartReader()
	if err != nil {
		kratosHTTP.DefaultErrorEncoder(w, r, biz.ErrVideoInvalidArgument)
		return
	}

	metadata := make(map[string]string, 3)
	for {
		multipartPart, nextErr := multipartReader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			kratosHTTP.DefaultErrorEncoder(w, r, biz.ErrVideoInvalidArgument)
			return
		}
		if nextErr != nil {
			var maxBytesError *http.MaxBytesError
			if errors.As(nextErr, &maxBytesError) {
				kratosHTTP.DefaultErrorEncoder(w, r, biz.ErrVideoUploadTooLarge)
			} else {
				kratosHTTP.DefaultErrorEncoder(w, r, biz.ErrVideoUploadInterrupted)
			}
			return
		}

		if multipartPart.FormName() != "file" {
			if err := readUploadField(multipartPart, metadata); err != nil {
				kratosHTTP.DefaultErrorEncoder(w, r, biz.ErrVideoInvalidArgument)
				return
			}
			continue
		}
		if !strings.EqualFold(filepath.Ext(multipartPart.FileName()), ".mp4") || strings.TrimSpace(metadata["title"]) == "" {
			kratosHTTP.DefaultErrorEncoder(w, r, biz.ErrVideoInvalidArgument)
			return
		}

		upload := &biz.VideoUploadInput{
			OwnerID:     userID,
			Title:       metadata["title"],
			Description: metadata["description"],
			Tags:        splitTags(metadata["tags"]),
			Content:     newIdleDeadlineReader(multipartPart, w, h.idleTimeout),
		}
		result, err := h.videoUsecase.UploadVideo(r.Context(), upload)
		if err != nil {
			kratosHTTP.DefaultErrorEncoder(w, r, err)
			return
		}
		bvid := result.VideoID.BVID()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true, "bvid": bvid,
			"manifestUrl": result.ManifestURL, "videoUrl": "/video/" + bvid,
		})
		return
	}
}

// Read extends the idle deadline and then reads the next bytes from the multipart file part.
func (r *idleDeadlineReader) Read(buffer []byte) (int, error) {
	_ = r.controller.SetReadDeadline(time.Now().Add(r.timeout))
	return r.source.Read(buffer)
}

// readUploadField reads one bounded metadata field from the multipart request.
func readUploadField(part *multipart.Part, metadata map[string]string) error {
	if part.FormName() != "title" && part.FormName() != "description" && part.FormName() != "tags" {
		return nil
	}
	value, err := io.ReadAll(io.LimitReader(part, 64*1024+1))
	if err != nil || len(value) > 64*1024 {
		return biz.ErrVideoInvalidArgument
	}
	metadata[part.FormName()] = string(value)
	return nil
}

// splitTags accepts comma or newline separated tags from the upload form.
func splitTags(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '，' || r == '\n'
	})
}
