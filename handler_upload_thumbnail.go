package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"

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

	// TODO: implement the upload here
	const maxMemory int = 10 << 20
	err = r.ParseMultipartForm(int64(maxMemory))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse multi part form file", err)
		return
	}

	file, fileHeader, err := r.FormFile("thumbnail")
	defer file.Close()
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable parse form file", err)
		return
	}
	mediaType := fileHeader.Header.Get("Content-Type")
	b, err := io.ReadAll(file)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable read form file", err)
		return
	}

	imgString := base64.StdEncoding.EncodeToString(b)
	dataUrl := fmt.Sprintf("data:%s;base64,%s", mediaType, imgString)
	videoData, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to get video from id", err)
		return
	}

	if videoData.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "You are not authorized to upload a thumbnail", err)
		return
	}

	videoData.ThumbnailURL = &dataUrl
	cfg.db.UpdateVideo(videoData)

	respondWithJSON(w, http.StatusOK, videoData)
}
