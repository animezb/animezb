package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"
)

type searchHits struct {
	Total int64             `json:"total"`
	Hits  []json.RawMessage `json:"hits"`
}

type searchResponse struct {
	Took int64      `json:"took"`
	Hits searchHits `json:"hits"`
}

type searchField struct {
	Size       []int64   `json:"size"`
	Complete   []int     `json:"complete"`
	Subject    []string  `json:"subject"`
	Poster     []string  `json:"poster"`
	Length     []int     `json:"length"`
	Filename   []string  `json:"filename"`
	Date       []string  `json:"date"`
	Group      []string  `json:"group"`
	Completion []float64 `json:"completion"`
}

type searchHit struct {
	Id     string      `json:"_id"`
	Fields searchField `json:"fields"`
}

type searchResult struct {
	Name            string
	Subject         string
	UploadId        string
	Size            string
	Bytes           int64
	CompletedParts  string
	TotalParts      string
	Completion      string
	CompletionClass string
	Category        string
	Age             string
	Types           []string
	ExtTypes        string
	Date            string
	FullGroup       string
	Group           string
	Poster          string
}

type searchResults struct {
	Query        string
	Category     string
	CategoryName string
	Results      []searchResult
	Pagination   []searchPages
	Page         string
	PrevPage     string
	NextPage     string
	LastPage     string
	UrlPath      func(string) string
}

type searchPages struct {
	Page     string
	Disabled bool
}

func urlPath(s string) string {
	return strings.Replace(url.QueryEscape(s), "+", "%20", -1)
}

func search(ctx *context, res http.ResponseWriter, req *http.Request) {
	req.ParseForm()
	var searchQuery string
	var category string
	var page int
	if q, ok := req.Form["q"]; ok {
		if q[0] == "" {
			searchQuery = "*"
		} else {
			searchQuery = q[0]
		}
	} else {
		switch req.URL.Path {
		case "/rss":
			searchQuery = "*"
		}
	}
	_, nocomp := req.Form["nocomp"]
	category = req.FormValue("cat")
	categoryName := "All"
	switch category {
	case "anime":
		categoryName = "Anime"
	default:
		category = ""
	}
	if n, err := strconv.Atoi(req.FormValue("p")); err == nil {
		page = n - 1
		if n < 0 {
			n = 0
		}
	} else {
		page = 0
	}
	if searchQuery == "" {
		if f, err := ctx.HtmlDir.Open("/home.html"); err == nil {
			defer f.Close()
			if fi, err := f.Stat(); err != nil {
				panic("Failed to stat index.html file.")
			} else {
				mod := fi.ModTime()
				http.ServeContent(res, req, "/index.html", mod, f)
			}
		}
	} else {
		res.Header().Set("Content-Type", "text/html")
		sResults, total := searchBackend(ctx, searchQuery, page, 200, !nocomp)
		lastpage := total/200 + 1
		results := searchResults{
			Query:        searchQuery,
			Category:     category,
			CategoryName: categoryName,
			Results:      sResults,
			Pagination:   pagination(page, int(lastpage)),
			Page:         strconv.Itoa(page + 1),
			PrevPage:     strconv.Itoa(page),
			NextPage:     strconv.Itoa(page + 2),
			LastPage:     strconv.Itoa(int(lastpage)),
			UrlPath:      urlPath,
		}
		t, err := template.New("results.html").ParseFiles("./www/results.html")
		if err == nil {
			res.WriteHeader(200)
			if err := t.Execute(res, results); err != nil {
				panic(err)
			}
		} else {
			panic(err)
		}
		return
	}
}

func pagination(page int, totalPages int) []searchPages {
	startPage := page - 4
	if page < 5 {
		startPage = 1
	}
	endPage := startPage + 8
	if totalPages < endPage {
		endPage = totalPages
	}
	diff := startPage - endPage + 8
	if startPage-diff > 0 {
		startPage -= diff
	}

	sp := make([]searchPages, 0, 12)

	if startPage > 1 {
		sp = append(sp, searchPages{Page: "1", Disabled: false}, searchPages{Page: "...", Disabled: true})
	}
	for i := startPage; i <= endPage; i++ {
		sp = append(sp, searchPages{Page: strconv.Itoa(i), Disabled: false})
	}

	if endPage < totalPages {
		sp = append(sp, searchPages{Page: "...", Disabled: true}, searchPages{Page: strconv.Itoa(totalPages), Disabled: false})
	}
	return sp
}

func searchBackend(ctx *context, searchQuery string, page int, length int, onlycomplete bool) ([]searchResult, int64) {
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"query_string": map[string]interface{}{
				"query":            searchQuery,
				"default_operator": "AND",
			},
		},
		"from": page * length,
		"size": length,
		"sort": []map[string]string{
			map[string]string{
				"date": "desc",
			},
		},
		"fields": "*",
	}
	if onlycomplete {
		query["filter"] = map[string]interface{}{
			"range": map[string]interface{}{
				"completion": map[string]interface{}{
					"gte": .9,
				},
			},
		}
	}
	b, err := json.Marshal(query)
	if err != nil {
		panic(err)
	}
	//{"query":{"fields":"*","simple_query_string":{"default_operator":"AND","query":"Horrible"},"size":200,"sort":[{"date":"desc"}]}}

	reader := bytes.NewReader(b)
	newReq, err := http.NewRequest("POST", fmt.Sprintf("http://%s:%d/nzb/upload/_search", ctx.EsHost, ctx.EsPort), reader)
	newReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(newReq)
	if err != nil {
		panic(err)
	}
	defer io.Copy(ioutil.Discard, resp.Body)
	defer resp.Body.Close()
	decoder := json.NewDecoder(resp.Body)
	var esResp searchResponse
	err = decoder.Decode(&esResp)
	if err != nil {
		panic(err)
	}
	results := make([]searchResult, len(esResp.Hits.Hits))
	for idx, hit := range esResp.Hits.Hits {
		var typesMap map[string]interface{}
		var parsedHit searchHit
		sr := searchResult{}
		json.Unmarshal(hit, &parsedHit)
		json.Unmarshal(hit, &typesMap)
		if len(typesMap) == 0 {
			continue
		}
		sr.Name = strings.TrimSuffix(parsedHit.Fields.Filename[0], ".")
		sr.Subject = parsedHit.Fields.Subject[0]
		sr.Poster = parsedHit.Fields.Poster[0]
		sr.UploadId = parsedHit.Id
		sr.Size = ByteSize(parsedHit.Fields.Size[0]).String()
		sr.Bytes = parsedHit.Fields.Size[0]
		if parsedHit.Fields.Complete[0] == parsedHit.Fields.Length[0] {
			sr.Completion = "100%"
		} else {
			sr.Completion = fmt.Sprintf("%0.2f%%", parsedHit.Fields.Completion[0]*100)
			sr.CompletionClass = "text-danger"
		}
		sr.CompletedParts = strconv.Itoa(parsedHit.Fields.Complete[0])
		sr.TotalParts = strconv.Itoa(parsedHit.Fields.Length[0])
		t, _ := time.Parse(time.RFC3339, parsedHit.Fields.Date[0])
		d := time.Now().Sub(t)
		if d.Minutes() < 90 {
			sr.Age = fmt.Sprintf("%0.0fm", d.Minutes())
		} else if d.Hours() < 12 {
			sr.Age = fmt.Sprintf("%0.0fh", d.Hours())
		} else {
			sr.Age = fmt.Sprintf("%0.0fd", d.Hours()/24)
		}
		switch parsedHit.Fields.Group[0] {
		case "alt.binaries.anime", "alt.binaries.multimedia.anime", "alt.binaries.multimedia.anime.repost", "alt.binaries.multimedia.anime.highspeed":
			sr.Category = "anime"
		default:
			sr.Category = "anime"
		}
		if len(parsedHit.Fields.Group) > 0 {
			sr.Group = parsedHit.Fields.Group[0]
		}
		sr.Date = t.Format(time.UnixDate)
		sr.Types = make([]string, 0, 4)
		keys := make([]string, 0, 4)
		for k, _ := range typesMap["fields"].(map[string]interface{}) {
			if strings.HasPrefix(k, "types.") {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := (typesMap["fields"].(map[string]interface{}))[k]
			if strings.HasPrefix(k, "types.") {
				ext := strings.TrimPrefix(k, "types.")
				sr.Types = append(sr.Types, fmt.Sprintf("%0.0f %s", (v.([]interface{}))[0], ext))
			}
		}
		sr.ExtTypes = strings.Join(sr.Types, ", ")
		sort.Strings(parsedHit.Fields.Group)
		sr.FullGroup = strings.Join(parsedHit.Fields.Group, ", ")
		results[idx] = sr

	}
	return results, esResp.Hits.Total
}
