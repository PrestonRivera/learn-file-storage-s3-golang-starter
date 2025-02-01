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

	// Parse the form data
	const maxMemory = 10 << 20
	if err := r.ParseMultipartForm(maxMemory); err != nil {
		respondWithError(w, http.StatusBadRequest, "Failed to Parse the form data", err)
		return
	}

	// Get the image data from the form
	file, fileHeader, err := r.FormFile("thumbnail"); 
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Failed to get image data", err)
		return
	}
	defer file.Close()

	headerVal := fileHeader.Header.Get("Content-Type")

	mediaType, _, err := mime.ParseMediaType(headerVal)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to get the media type", err)
		return
	}

	if mediaType != "image/jpeg" && mediaType != "image/png" {
		respondWithError(w, http.StatusBadRequest, "Invalid media type", nil)
		return
	}

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Failed to get video from database", err)
		return
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized user", nil)
		return
	}

	slice := make([]byte, 32)
	_, err = rand.Read(slice)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to create URL", nil)
		return
	}
	URLString := base64.RawURLEncoding.EncodeToString(slice)

	splitMediaType := strings.Split(mediaType, "/")
	fileName := fmt.Sprintf("%s.%s", URLString, splitMediaType[1])

	filePath := filepath.Join(cfg.assetsRoot, fileName)
	
	newFile, err := os.Create(filePath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to create new file path", err)
		return
	}
	defer newFile.Close()

	_, err = io.Copy(newFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to copy multipart file to new file", err)
		return
	}

	thumbnailURL := fmt.Sprintf("http://localhost:%s/assets/%s", cfg.port, fileName)

	video.ThumbnailURL = &thumbnailURL
	video.UpdatedAt = time.Now()

	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Failed to update video", err)
		return
	}
	respondWithJSON(w, http.StatusOK, video)
}
