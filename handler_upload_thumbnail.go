package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

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

	const maxMemory int = 10 << 20
	err = r.ParseMultipartForm(int64(maxMemory))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse multi part form file", err)
		return
	}

	file, fileHeader, err := r.FormFile("thumbnail")
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

	if mediaType != "image/jpeg" && mediaType != "image/png" {
		respondWithError(w, http.StatusBadRequest, "Must be a jpeg or png", nil)
		return
	}

	ext := strings.Split(mediaType, "/")[1]
	b := make([]byte, 32)
	rand.Read(b)
	name := base64.RawURLEncoding.EncodeToString(b)
	fileName := fmt.Sprintf("%v.%s", name, ext)
	thumbPath := filepath.Join(cfg.assetsRoot, fileName)
	newFile, err := os.Create(thumbPath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not create new file", err)
	}
	defer newFile.Close()

	_, err = io.Copy(newFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error saving file", err)
		return
	}

	baseURL := fmt.Sprintf("http://localhost:%s", cfg.port)
	thumbUrl := fmt.Sprintf("%s/assets/%s", baseURL, fileName)

	videoData, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to get video from id", err)
		return
	}

	if videoData.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "You are not authorized to upload a thumbnail", nil)
		return
	}

	videoData.ThumbnailURL = &thumbUrl
	cfg.db.UpdateVideo(videoData)

	respondWithJSON(w, http.StatusOK, videoData)
}
