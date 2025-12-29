package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	videoIdStr := r.PathValue("videoID")
	videoId, err := uuid.Parse(videoIdStr)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid video id", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userId, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading video file for ", videoId, "by user ", userId)

	videoDbResp, err := cfg.db.GetVideo(videoId)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Something went wrong", err)
		return
	}

	if videoDbResp.UserID != userId {
		respondWithError(w, http.StatusUnauthorized, "not owner", nil)
		return
	}

	const maxMemory = 1 << 30
	r.ParseMultipartForm(maxMemory)
	videoFile, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Something went wrong", err)
		return
	}
	defer videoFile.Close()

	mediaType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil || mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Something went wrong", err)
		return
	}

	fileTemp, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Something went wrong", err)
		return
	}
	defer os.Remove(fileTemp.Name())
	defer fileTemp.Close()

	_, err = io.Copy(fileTemp, videoFile)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Something went wrong", err)
		return
	}

	_, err = fileTemp.Seek(0, io.SeekStart)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Something went wrong", err)
		return
	}

	ration, err := getVideoAspectRation(fileTemp.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Something went wrong", err)
		return
	}

	key := make([]byte, 32)
	rand.Read(key)

	randFileName := ration + "/" + base64.URLEncoding.EncodeToString(key) + ".mp4"

	_, err = cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &randFileName,
		Body:        fileTemp,
		ContentType: &mediaType,
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Something went wrong", err)
		return
	}

	videoUrl := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, randFileName)

	err = cfg.db.UpdateVideo(database.Video{
		ID:                videoId,
		CreatedAt:         videoDbResp.CreatedAt,
		UpdatedAt:         time.Now().UTC(),
		ThumbnailURL:      videoDbResp.ThumbnailURL,
		VideoURL:          &videoUrl,
		CreateVideoParams: videoDbResp.CreateVideoParams,
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Something went wrong", err)
		return
	}
}

func getVideoAspectRation(path string) (string, error) {
	execCom := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", path)
	var out bytes.Buffer
	execCom.Stdout = &out
	err := execCom.Run()
	if err != nil {
		return "", err
	}

	type stream struct {
		Width  int
		Height int
	}

	type streams struct {
		Streams []stream
	}
	var jsonRes streams
	err = json.Unmarshal(out.Bytes(), &jsonRes)
	if err != nil {
		return "", err
	}

	width := jsonRes.Streams[0].Width
	height := jsonRes.Streams[0].Height

	//calculate this just because the exercise asked it...
	gcdRes := gcd(jsonRes.Streams[0].Width, jsonRes.Streams[0].Height)

	ratioWidth := width / gcdRes
	rationHeight := height / gcdRes

	if ratioWidth < rationHeight {
		return "portrait", nil
	} else if ratioWidth > rationHeight {
		return "landscape", nil
	} else {
		return "other", nil
	}
}

func gcd(nr1, nr2 int) int {
	for nr2 != 0 {
		nr1, nr2 = nr2, nr1%nr2
	}

	return nr1
}
