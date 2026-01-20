package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	const maxMemory int64 = 10 << 30
	r.Body = http.MaxBytesReader(w, r.Body, maxMemory)
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

	videoData, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to get video from id", err)
		return
	}
	if videoData.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "You are not authorized to do this", nil)
		return
	}

	fmt.Println("uploading video", videoID, "by user", userID)

	file, fileHeader, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable parse form file", err)
		return
	}

	defer file.Close()
	contentTypeHeader := fileHeader.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentTypeHeader)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Could not parse media type from header", err)
		return
	}

	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Must be an mp4", nil)
		return
	}

	b := make([]byte, 32)
	rand.Read(b)
	key := base64.RawURLEncoding.EncodeToString(b)
	ext := strings.Split(mediaType, "/")[1]
	keyExt := fmt.Sprintf("%s.%s", key, ext)

	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not create new file", err)
		return
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	_, err = io.Copy(tempFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error saving file", err)
		return
	}

	aspectRatio, err := getVideoAspectRatio(tempFile.Name())
	if err != nil {
		log.Printf("getVideoAspectRatio failed: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Could not get file aspect ratio", err)
		return
	}

	quickStartVideo, err := processVideoForFastStart(tempFile.Name())
	if err != nil {
		log.Printf("processVideoForFastStart failed: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Could not process video", err)
		return
	}

	defer os.Remove(quickStartVideo)
	processedFile, err := os.Open(quickStartVideo)
	if err != nil {
		log.Printf("os.Open(quickStartVideo) failed: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Could not open processed video", err)
		return
	}

	defer processedFile.Close()
	var layout string
	if aspectRatio == "16:9" {
		layout = "landscape"
	} else if aspectRatio == "9:16" {
		layout = "portrait"
	} else {
		layout = "other"
	}

	fullKey := fmt.Sprintf("%s/%s", layout, keyExt)

	_, err = cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:      aws.String(cfg.s3Bucket),
		Key:         aws.String(fullKey),
		Body:        processedFile,
		ContentType: aws.String(mediaType),
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error uploading file to S3", err)
		return
	}

	videoURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, fullKey)
	videoData.VideoURL = &videoURL
	err = cfg.db.UpdateVideo(videoData)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not update video", err)
		return
	}
	respondWithJSON(w, http.StatusOK, videoData)
}
