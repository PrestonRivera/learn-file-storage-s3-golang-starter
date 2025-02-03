package main

import (
	"crypto/rand"
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
	videoIdString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIdString)
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
		respondWithError(w, http.StatusUnauthorized, "Couldn't validat JWT", err)
		return
	}

	const maxUpload = 1 << 30
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to find video", err)
		return
	}

	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Invalid user", nil)
		return
	}

	file, fileHeader, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Failed to get video data", err)
		return
	}
	defer file.Close()

	mediaType,  _,  err := mime.ParseMediaType(fileHeader.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to get media type", err)
		return
	}

	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Invalid media type", nil)
		return
	}

	tmpFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to create temp file", err)
		return
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if _, err = io.Copy(tmpFile, file); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to copy file to temp file", err)
		return
	}

	ratio, err := getVideoAspectRatio(tmpFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to get video aspect ratio", err)
		return
	}

	var prefix string
	switch ratio {
	case "16:9":
		prefix = "landscape/"
	case "9:16":
		prefix = "portrait/"
	case "other":
		prefix = "other/"
	}

	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to reset file pointer", err)
		return
	}

	fileKey := make([]byte, 32)
	if _, err = rand.Read(fileKey); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to create file key", err)
		return
	}

	encodedFileKey := fmt.Sprintf("%x", fileKey)

	splitMT := strings.Split(mediaType, "/")

	fileName := fmt.Sprintf("%s%s.%s",prefix, encodedFileKey, splitMT[1])

	processedVid, err := processVideoForFastStart(tmpFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to create fast start video", err)
		return
	}

	processedFile, err := os.Open(processedVid)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to read fast start file", err)
		return
	}
	defer processedFile.Close()

	_, err = cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket: &cfg.s3Bucket,
		Key: &fileName,
		Body: processedFile,
		ContentType: &mediaType,
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to add object to s3", err)
		return
	}

	videoURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, fileName)

	video.VideoURL = &videoURL
	video.UpdatedAt = time.Now()

	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to update video URL", err)
		return
	}
}
