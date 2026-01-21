package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
)

func getVideoAspectRatio(filePath string) (string, error) {
	var result struct {
		Streams []struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"streams"`
	}
	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	b := &bytes.Buffer{}
	cmd.Stdout = b
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	err := cmd.Run()
	if err != nil {
		log.Printf("ffprobe failed for %s: %v\nstderr: %s", filePath, err, stderr.String())
		return "", err
	}

	err = json.Unmarshal(b.Bytes(), &result)
	if err != nil {
		log.Printf("unmarshal failed for %s: %v\noutput: %s", filePath, err, b.String())
		return "", err
	}
	if len(result.Streams) == 0 {
		log.Printf("no streams found for %s; output: %s", filePath, b.String())
		return "", errors.New("No streams for path")
	}

	width := result.Streams[0].Width
	height := result.Streams[0].Height
	if width == 16*height/9 {
		return "16:9", nil
	} else if height == 16*width/9 {
		return "9:16", nil
	}
	return "other", nil
}

func processVideoForFastStart(filePath string) (string, error) {
	outputFilePath := fmt.Sprintf("%s.processing", filePath)
	cmd := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", outputFilePath)
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	err := cmd.Run()
	if err != nil {
		log.Printf("ffmpeg failed for %s: %v\nstderr: %s", filePath, err, stderr.String())
		return "", err
	}

	return outputFilePath, nil
}

func generatePresignedURL(s3Client *s3.Client, bucket, key string, expireTime time.Duration) (string, error) {
	preSignedClient := s3.NewPresignClient(s3Client)
	result, err := preSignedClient.PresignGetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	},
		s3.WithPresignExpires(expireTime),
	)
	if err != nil {
		return "", err
	}
	return result.URL, nil
}

func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (database.Video, error) {

	if video.VideoURL == nil {
		return video, nil
	}

	parts := strings.Split(*video.VideoURL, ",")
	if len(parts) != 2 {
		return video, nil
	}
	bucket, key := parts[0], parts[1]
	presignedURL, err := generatePresignedURL(cfg.s3Client, bucket, key, time.Minute*5)
	if err != nil {
		return video, err
	}
	video.VideoURL = &presignedURL
	return video, nil
}
