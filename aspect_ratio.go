package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
)


type VideoInfo struct {
	Streams []struct {
		Width 	int `json:"width"`
		Height	int `json:"height"`
	} `json:"streams"`
}


//
func getVideoAspectRatio(filepath string) (string, error) {
	cmd := exec.Command("ffprobe", "-v", "error" , "-print_format", "json", "-show_streams", filepath)
	var result bytes.Buffer
	cmd.Stdout = &result
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("failed to run command: %v", err)
	}

	params := VideoInfo{}
	err = json.Unmarshal(result.Bytes(), &params)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal data: %v", err)
	}

	if len(params.Streams) == 0 {
		return "", fmt.Errorf("no streams in video")
	}

	width := params.Streams[0].Width
	height := params.Streams[0].Height

	ratio := float64(width) / float64(height)

	if ratio >= 1.7 && ratio <= 1.8 {
		return "16:9", nil
	} else if ratio >= 0.5 && ratio <= 0.6 {
		return "9:16", nil 
	}
	return "other", nil
}