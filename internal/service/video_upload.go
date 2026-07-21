package service

import (
	"bytes"
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
	appMiddleware "bilibili-lite/internal/middleware"

	kratosHTTP "github.com/go-kratos/kratos/v3/transport/http"
)

// VideoUploadHTTPHandler streams one multipart MP4 directly into the media usecase.
type VideoUploadHTTPHandler struct {
	videoUsecase  *biz.VideoUsecase
	maxBytes      int64
	maxCoverBytes int64
	idleTimeout   time.Duration
}

// idleDeadlineReader refreshes the HTTP read deadline whenever upload data is consumed.
type idleDeadlineReader struct {
	source     io.Reader
	controller *http.ResponseController
	timeout    time.Duration
}

// NewVideoUploadHTTPHandler creates the authenticated HTTP adapter for streaming MP4 uploads.
func NewVideoUploadHTTPHandler(videoUsecase *biz.VideoUsecase, dataConfig *conf.Data) *VideoUploadHTTPHandler {
	return &VideoUploadHTTPHandler{
		videoUsecase:  videoUsecase,
		maxBytes:      dataConfig.GetMedia().GetMaxUploadBytes(),
		maxCoverBytes: dataConfig.GetMedia().GetMaxCoverBytes(),
		idleTimeout:   dataConfig.GetMedia().GetUploadIdleTimeout().AsDuration(),
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
	userID, err := appMiddleware.RequireUserID(r.Context())
	if err != nil {
		kratosHTTP.DefaultErrorEncoder(w, r, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBytes+h.maxCoverBytes+1024*1024)
	multipartReader, err := r.MultipartReader()
	if err != nil {
		kratosHTTP.DefaultErrorEncoder(w, r, biz.ErrVideoInvalidArgument)
		return
	}

	var cover []byte
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

		if multipartPart.FormName() == "cover" {
			if !isSupportedCover(multipartPart.FileName()) {
				kratosHTTP.DefaultErrorEncoder(w, r, biz.ErrVideoInvalidArgument)
				return
			}
			cover, err = readCover(multipartPart, h.maxCoverBytes)
			if err != nil {
				kratosHTTP.DefaultErrorEncoder(w, r, err)
				return
			}
			continue
		}
		if multipartPart.FormName() != "file" {
			continue
		}
		if !strings.EqualFold(filepath.Ext(multipartPart.FileName()), ".mp4") {
			kratosHTTP.DefaultErrorEncoder(w, r, biz.ErrVideoInvalidArgument)
			return
		}

		upload := &biz.VideoUploadInput{
			OwnerID: userID,
			Content: newIdleDeadlineReader(multipartPart, w, h.idleTimeout),
		}
		if len(cover) > 0 {
			upload.Cover = bytes.NewReader(cover)
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
			"status": result.Status, "manifestUrl": result.ManifestURL,
			"coverUrl": result.CoverURL, "videoUrl": "/video/" + bvid,
		})
		return
	}
}

// Read extends the idle deadline and then reads the next bytes from the multipart file part.
func (r *idleDeadlineReader) Read(buffer []byte) (int, error) {
	_ = r.controller.SetReadDeadline(time.Now().Add(r.timeout))
	return r.source.Read(buffer)
}

func readCover(part *multipart.Part, limit int64) ([]byte, error) {
	value, err := io.ReadAll(io.LimitReader(part, limit+1))
	if err != nil {
		return nil, biz.ErrVideoUploadInterrupted
	}
	if int64(len(value)) > limit {
		return nil, biz.ErrVideoUploadTooLarge
	}
	return value, nil
}

func isSupportedCover(filename string) bool {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".jpg", ".jpeg", ".png", ".webp":
		return true
	default:
		return false
	}
}
