package main

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	const maxMemory = 10 << 20

	err = r.ParseMultipartForm(maxMemory)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse multipart form", err)
		return
	}
	defer r.MultipartForm.RemoveAll()

	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	contentType := header.Header.Get("Content-Type")

	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Content-Type", err)
		return
	}

	if mediaType != "image/jpeg" && mediaType != "image/png" {
		respondWithError(w, http.StatusUnsupportedMediaType, "Only JPEG and PNG images are supported.", err)
		return
	}

	dbVideo, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Couldn't get video", err)
		return
	}

	if userID != dbVideo.UserID {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate video ownership", err)
		return
	}

	fileMediaTypeStrings := strings.Split(mediaType, "/")
	fileExt := fileMediaTypeStrings[1]
	fileName := fmt.Sprintf("%s.%s", videoIDString, fileExt)
	filePath := filepath.Join(cfg.assetsRoot, fileName)

	newThumbnail, err := os.Create(filePath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "unable to create file", err)
		return
	}
	defer newThumbnail.Close()

	bytes, err := io.Copy(newThumbnail, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to copy file data", err)
		return
	}
	fmt.Println(bytes, "bytes has been successfully written")

	thumbnailURL := fmt.Sprintf("http://%s/assets/%s", r.Host, fileName)

	dbVideo.ThumbnailURL = &thumbnailURL
	dbVideo.UpdatedAt = time.Now()

	err = cfg.db.UpdateVideo(dbVideo)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to update video", err)
		return
	}

	respondWithJSON(w, http.StatusOK, dbVideo)
}
