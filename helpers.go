package main

import (
	"bytes"
	"encoding/json"
	"math"
	"os/exec"
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
