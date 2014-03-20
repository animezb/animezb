package main

import (
	"encoding/json"
	"github.com/codegangsta/martini"
	"net/http"
	"sort"
	"time"
)

type uploadInfo struct {
	Files []fileInfo `json:"files"`
}

type fileInfo struct {
	Date    string `json:"date"`
	Time    int64  `json:"time"`
	Subject string `json:"subject"`
	Parts   int    `json:"parts"`
	Length  int    `json:"length"`
	Size    string `json:"size"`
}

type uploadFiles []fileInfo

func (s uploadFiles) Len() int           { return len(s) }
func (s uploadFiles) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s uploadFiles) Less(i, j int) bool { return s[i].Subject < s[j].Subject }

func getUploadInfo(ctx *context, params martini.Params, res http.ResponseWriter, req *http.Request) {
	uploadId := params["nzbid"]
	files := getFiles(ctx, uploadId)
	r := uploadInfo{
		Files: make([]fileInfo, len(files)),
	}
	for idx, file := range files {
		fi := fileInfo{
			Date:    time.Unix(file.Date, 0).Format("2006-01-02"),
			Time:    file.Date,
			Subject: file.Subject,
			Parts:   file.Parts,
			Length:  file.Length,
			Size:    ByteSize(file.Bytes).String(),
		}
		r.Files[idx] = fi
	}
	sort.Sort(uploadFiles(r.Files))
	if output, err := json.Marshal(r); err == nil {
		res.Header().Set("Content-Type", "application/json")
		res.WriteHeader(200)
		res.Write(output)
	} else {
		panic(err)
	}
}
