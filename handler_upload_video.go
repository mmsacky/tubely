package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {

	const uploadLimit = 1 << 30
	r.Body = http.MaxBytesReader(w, r.Body, uploadLimit)

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

	fmt.Println("uploading video", videoID, "by user", userID)

	dbVideo, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Couldn't get video", err)
		return
	}

	if userID != dbVideo.UserID {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate video ownership", nil)
		return
	}

	videoFile, videoHeader, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer videoFile.Close()

	contentType := videoHeader.Header.Get("Content-Type")

	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Content-Type", err)
		return
	}

	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusUnsupportedMediaType, "Only MP4 videos are supported.", nil)
		return
	}

	tempVid, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "unable to create temporary file", err)
		return
	}
	defer os.Remove(tempVid.Name())
	defer tempVid.Close()

	_, err = io.Copy(tempVid, videoFile)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to copy file data", err)
		return
	}

	_, err = tempVid.Seek(0, io.SeekStart)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to restart video", err)
		return
	}

	fileMediaTypeStrings := strings.Split(mediaType, "/")
	fileExt := fileMediaTypeStrings[1]
	randBytes := make([]byte, 32)
	rand.Read(randBytes)
	bytesString := hex.EncodeToString(randBytes)
	fileName := fmt.Sprintf("%s.%s", bytesString, fileExt)

	var prefix string
	aspectRatio, err := getVideoAspectRatio(tempVid.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to get aspect ratio", err)
		return
	}
	switch aspectRatio {
	case "16:9":
		prefix = "landscape"
	case "9:16":
		prefix = "landscape"
	default:
		prefix = "other"
	}

	key := fmt.Sprintf("%s/%s", prefix, fileName)

	objParams := s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Body:        tempVid,
		Key:         &key,
		ContentType: &mediaType,
	}

	_, err = cfg.s3Client.PutObject(r.Context(), &objParams)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to upload to S3", err)
		return
	}

	newVideoURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, key)

	dbVideo.VideoURL = &newVideoURL
	dbVideo.UpdatedAt = time.Now()

	err = cfg.db.UpdateVideo(dbVideo)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to update video", err)
		return
	}

}
