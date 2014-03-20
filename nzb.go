package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"github.com/animezb/newsroverd/extract"
	"github.com/animezb/newsroverd/sinks/elasticsink"
	"github.com/codegangsta/martini"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
)

const (
	NZB_XMLNS = "http://www.newzbin.com/DTD/2003/nzb"
)

type nzb struct {
	name  xml.Name  `xml:"nzb"`
	Xmlns string    `xml:"xmlns,attr"`
	Head  []NzbMeta `xml:"head>meta"`
	Files []NzbFile `xml:"file"`
}

type NzbMeta struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",innerxml"`
}

type NzbFile struct {
	Id       string       `xml:"-"`
	Name     string       `xml:"-"`
	Parts    int          `xml:"-"`
	Length   int          `xml:"-"`
	Bytes    int64        `xml:"-"`
	Poster   string       `xml:"poster,attr"`
	Date     int64        `xml:"date,attr"`
	Subject  string       `xml:"subject,attr"`
	Groups   []string     `xml:"groups>group"`
	Segments []NzbSegment `xml:"segments>segment"`
}

type NzbSegment struct {
	Bytes     uint32 `xml:"bytes,attr"`
	Number    uint32 `xml:"number,attr"`
	MessageId string `xml:",innerxml"`
}

type esFileResp struct {
	Hits struct {
		Hits []struct {
			Id     string           `json:"_id"`
			Source elasticsink.File `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}

type esSegmentResp struct {
	Hits struct {
		Hits []struct {
			Id     string              `json:"_id"`
			Source elasticsink.Segment `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}

func gennzb(ctx *context, params martini.Params, res http.ResponseWriter, req *http.Request) {
	var uploads []string
	if req.Method == "GET" {
		uploads = []string{params["nzbid"]}
	} else if req.Method == "POST" {
		req.ParseForm()
		uploads = req.PostForm["nzb"]
	}
	nzbdl := nzb{
		Xmlns: NZB_XMLNS,
		Files: make([]NzbFile, 0, 16),
	}
	nzbName := params["nzbname"]
	if nzbName == "" && len(uploads) > 0 {
		nzbName = getName(ctx, uploads[0])
	}
	res.Header().Set("Content-Type", "text/html")
	for _, upload := range uploads {
		uploadFiles := getFiles(ctx, upload)
		for _, f := range uploadFiles {
			if nzbName == "" {
				nzbName = f.Name
			}
			f.Segments = getSegments(ctx, f.Id)
			nzbdl.Files = append(nzbdl.Files, f)
		}
	}
	if !strings.HasSuffix(nzbName, ".nzb") {
		nzbName += ".nzb"
	}
	if output, err := xml.Marshal(nzbdl); err == nil {
		res.Header().Set("Content-Type", "application/x-nzb")
		res.Header().Set("Content-Disposition", "attachment; filename=\""+nzbName+"\"")
		res.WriteHeader(200)
		res.Write([]byte(xml.Header))
		res.Write([]byte(`<!DOCTYPE nzb PUBLIC "-//newzBin//DTD NZB 1.0//EN" "http://www.newzbin.com/DTD/nzb/nzb-1.0.dtd">` + "\n"))
		res.Write(output)
	} else {
		panic(err)
	}
}

func getName(ctx *context, upload string) string {
	newReq, err := http.NewRequest("GET", fmt.Sprintf("http://%s:%d/nzb/upload/%s?_source_include=fileprefix", ctx.EsHost, ctx.EsPort, upload), nil)
	resp, err := http.DefaultClient.Do(newReq)
	if err != nil {
		panic(err)
	}
	defer io.Copy(ioutil.Discard, resp.Body)
	defer resp.Body.Close()
	decoder := json.NewDecoder(resp.Body)
	var esResp struct {
		Source struct {
			Name string `json:"fileprefix"`
		} `json:"_source"`
		Found bool `json:"found"`
	}
	err = decoder.Decode(&esResp)
	if err != nil {
		panic(err)
	}
	return strings.TrimSuffix(esResp.Source.Name, ".")
}

func getFiles(ctx *context, upload string) []NzbFile {
	query := map[string]interface{}{
		"filter": map[string]interface{}{
			"term": map[string]interface{}{
				"_routing": upload,
			},
		},
		"size": 16384,
	}
	b, err := json.Marshal(query)
	if err != nil {
		panic(err)
	}

	reader := bytes.NewReader(b)
	newReq, err := http.NewRequest("POST", fmt.Sprintf("http://%s:%d/nzb/file/_search?_source_exclude=segments", ctx.EsHost, ctx.EsPort), reader)
	newReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(newReq)
	if err != nil {
		panic(err)
	}
	defer io.Copy(ioutil.Discard, resp.Body)
	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)
	var esResp esFileResp
	err = decoder.Decode(&esResp)
	if err != nil {
		panic(err)
	}
	results := make([]NzbFile, len(esResp.Hits.Hits))
	for idx, hit := range esResp.Hits.Hits {
		nzbf := NzbFile{
			Id:      hit.Id,
			Name:    hit.Source.Filename,
			Poster:  hit.Source.Poster,
			Date:    hit.Source.Date.Unix(),
			Subject: ensureFirstPart(hit.Source.Subject),
			Groups:  hit.Source.Group,
			Parts:   hit.Source.Complete,
			Length:  hit.Source.Length,
			Bytes:   hit.Source.Size,
		}
		results[idx] = nzbf
	}
	return results
}

func ensureFirstPart(subject string) string {
	/* SabNzbd won't properly extract without this */
	part := extract.ExtractYencPart(subject)
	if part != 1 {
		length := extract.ExtractYencLength(subject)
		nsubject := strings.Replace(subject, fmt.Sprintf("(%d/%d)", part, length), fmt.Sprintf("(1/%d)", length), 1)
		if nsubject == subject {
			nsubject = strings.Replace(subject, fmt.Sprintf("(%02d/%d)", part, length), fmt.Sprintf("(01/%d)", length), 1)
		}
		if nsubject == subject {
			nsubject = strings.Replace(subject, fmt.Sprintf("(%03d/%d)", part, length), fmt.Sprintf("(001/%d)", length), 1)
		}
		if nsubject == subject {
			nsubject = strings.Replace(subject, fmt.Sprintf("(%04d/%d)", part, length), fmt.Sprintf("(001/%d)", length), 1)
		}
		subject = nsubject
	}
	return subject
}

func getSegments(ctx *context, fileId string) []NzbSegment {
	query := map[string]interface{}{
		"filter": map[string]interface{}{
			"term": map[string]interface{}{
				"_routing": fileId,
			},
		},
		"size": 65536,
	}
	b, err := json.Marshal(query)
	if err != nil {
		panic(err)
	}

	reader := bytes.NewReader(b)
	newReq, err := http.NewRequest("POST", fmt.Sprintf("http://%s:%d/nzb/segment/_search", ctx.EsHost, ctx.EsPort), reader)
	newReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(newReq)
	if err != nil {
		panic(err)
	}

	defer io.Copy(ioutil.Discard, resp.Body)
	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)
	var esResp esSegmentResp
	err = decoder.Decode(&esResp)
	if err != nil {
		panic(err)
	}
	results := make([]NzbSegment, len(esResp.Hits.Hits))
	for idx, hit := range esResp.Hits.Hits {
		nzbsg := NzbSegment{
			Bytes:     uint32(hit.Source.Bytes),
			Number:    uint32(hit.Source.Part),
			MessageId: strings.TrimSuffix(strings.TrimPrefix(hit.Source.MessageId, "<"), ">"),
		}
		results[idx] = nzbsg
	}
	return results
}
