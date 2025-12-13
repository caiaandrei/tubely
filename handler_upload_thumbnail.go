package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
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
	imgType := header.Header.Get("Content-Type")
	data, err := io.ReadAll(file)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to read the file", err)
		return
	}

	videoDbResp, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Something went wrong", err)
		return
	}

	if videoDbResp.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "not owner", nil)
	}

	base64Data := base64.StdEncoding.EncodeToString(data)
	thumbUrl := fmt.Sprintf("data:%s;base64,%s", imgType, base64Data)
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
