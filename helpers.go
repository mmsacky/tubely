package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
)

func getVideoAspectRatio(filePath string) (string, error) {
	videoData := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)

	var buf bytes.Buffer
	videoData.Stdout = &buf

	err := videoData.Run()
	if err != nil {
		return "", err
	}

	type VideoInfo struct {
		Streams []struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"streams"`
	}

	var videoInfoParams VideoInfo
	err = json.Unmarshal(buf.Bytes(), &videoInfoParams)
	if err != nil {
		return "", err
	}

	width := videoInfoParams.Streams[0].Width
	height := videoInfoParams.Streams[0].Height

	const tolerance = 0.05
	ratio := float64(width) / float64(height)

	switch {
	case math.Abs(ratio-16.0/9.0) < tolerance:
		return "16:9", nil
	case math.Abs(ratio-9.0/16.0) < tolerance:
		return "9:16", nil
	default:
		return "other", nil
	}

}

func processVideoForFastStart(filePath string) (string, error) {

	outputFilePath := fmt.Sprintf("%s.processing", filePath)

	cmd := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", outputFilePath)

	err := cmd.Run()
	if err != nil {
		return "", err
	}

	return outputFilePath, nil
}

func generatePresignedURL(s3Client *s3.Client, bucket, key string, expireTime time.Duration) (string, error) {
	s3PresignClient := s3.NewPresignClient(s3Client)

	objectInput := s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}

	presignedReq, err := s3PresignClient.PresignGetObject(context.Background(), &objectInput, s3.WithPresignExpires(expireTime))
	if err != nil {
		return "", err
	}

	return presignedReq.URL, nil
}

func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (database.Video, error) {
	expireTime := 5 * time.Minute
	if video.VideoURL == nil {
		return video, errors.New("database video doesn't have a URL")
	}

	results := strings.Split(*video.VideoURL, ",")

	bucket := results[0]
	key := results[1]

	preSignedURL, err := generatePresignedURL(cfg.s3Client, bucket, key, expireTime)
	if err != nil {
		return video, err
	}

	video.VideoURL = &preSignedURL

	return video, nil
}
