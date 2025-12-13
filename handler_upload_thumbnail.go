package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
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

	const maxMemory = 10 << 20
	r.ParseMultipartForm(maxMemory)
	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse from file", err)
		return
	}
	defer file.Close()
	contentType := header.Header.Get("Content-Type")
	splitedTypes := strings.Split(contentType, "/")
	if len(splitedTypes) == 0 {
		respondWithError(w, http.StatusBadRequest, "Something went wrong", fmt.Errorf("Content-Type cannot be empty"))
		return
	}
	imgType := splitedTypes[len(splitedTypes)-1]

	videoDbResp, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Something went wrong", err)
		return
	}

	if videoDbResp.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "not owner", nil)
	}
	imgPath := filepath.Join(cfg.assetsRoot, videoID.String()) + "." + imgType
	diskFile, err := os.Create(imgPath)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Something went wrong", err)
		return
	}
	defer diskFile.Close()
	_, err = io.Copy(diskFile, file)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Something went wrong", err)
		return
	}

	thumbUrl := "http://localhost:8091/" + imgPath
	video := database.Video{
		ID:                videoDbResp.ID,
		CreatedAt:         videoDbResp.CreatedAt,
		UpdatedAt:         time.Now().UTC(),
		ThumbnailURL:      &thumbUrl,
		VideoURL:          videoDbResp.VideoURL,
		CreateVideoParams: videoDbResp.CreateVideoParams,
	}
	err = cfg.db.UpdateVideo(video)

	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Something went wrong", err)
		return
	}
	fmt.Println(video)
	respondWithJSON(w, http.StatusOK, video)
}
