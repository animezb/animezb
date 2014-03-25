package main

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	ATOM_XLMNS = "http://www.w3.org/2005/Atom"
)

type rss struct {
	name    xml.Name   `xml:"rss"`
	Xmlns   string     `xml:"xmlns:atom,attr"`
	Version string     `xml:"version,attr"`
	Channel RssChannel `xml:"channel"`
}

type RssChannel struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	Language    string `xml:"language"`
	AtomLink    struct {
		Href string `xml:"href,attr"`
		Rel  string `xml:"rel,attr"`
		Type string `xml:"type,attr"`
	} `xml:"atom:link"`
	Items []RssItem `xml:"item"`
}

type RssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	Category    string `xml:"category"`
	PubDate     string `xml:"pubDate"`
	Enclosure   struct {
		Url    string `xml:"url,attr"`
		Length int64  `xml:"length,attr"`
		Type   string `xml:"type,attr"`
	} `xml:"enclosure"`
	Guid struct {
		Perma string `xml:"isPermalink,attr"`
		Guid  string `xml:",innerxml"`
	} `xml:"guid"`
}

func genrss(ctx *context, res http.ResponseWriter, req *http.Request) {
	req.ParseForm()
	var searchQuery string
	//var category string
	var max int
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
	/*
		category = req.FormValue("cat")
		categoryName := "All"
		switch category {
		case "anime":
			categoryName = "Anime"
		default:
			category = ""
		}
	*/
	if n, err := strconv.Atoi(req.FormValue("max")); err == nil {
		max = n
	} else {
		max = 50
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
		sResults, _ := searchBackend(ctx, searchQuery, 0, max, true)
		protocol := req.URL.Scheme + "://"
		hostname := req.Host
		feed := rss{
			Xmlns:   ATOM_XLMNS,
			Version: "2.0",
			Channel: RssChannel{
				Title:       searchQuery + " &mdash; Animezb",
				Link:        protocol + hostname,
				Description: "Usenet Indexer for Japanese Media",
				Language:    "en-us",
			},
		}
		feed.Channel.AtomLink.Href = protocol + hostname + req.URL.String()
		feed.Channel.AtomLink.Rel = "self"
		feed.Channel.AtomLink.Type = "application/rss+xml"
		feed.Channel.Items = make([]RssItem, len(sResults))

		for idx, res := range sResults {
			var postCat string
			switch res.Group {
			case "alt.binaries.anime", "alt.binaries.multimedia.anime", "alt.binaries.multimedia.anime.repost", "alt.binaries.multimedia.anime.highspeed":
				postCat = "Anime"
			default:
				postCat = "Anime"
			}
			pDate, _ := time.Parse(time.UnixDate, res.Date)
			item := RssItem{
				Title:       res.Name,
				Link:        protocol + hostname + "/nzb/" + res.UploadId,
				Description: formatRssDesc(res),
				Category:    postCat,
				PubDate:     pDate.Format(time.RFC1123Z),
			}
			item.Enclosure.Url = protocol + hostname + "/nzb/" + res.UploadId + "/" + strings.Replace(url.QueryEscape(res.Name), "+", "%20", -1) + ".nzb"
			item.Enclosure.Length = res.Bytes
			item.Enclosure.Type = "application/x-nzb"
			item.Guid.Guid = protocol + hostname + "/nzb/" + res.UploadId
			item.Guid.Perma = "false"
			feed.Channel.Items[idx] = item
		}
		if output, err := xml.Marshal(feed); err == nil {
			res.Header().Set("Content-Type", "text/xml; charset=utf-8")
			res.WriteHeader(200)
			res.Write([]byte(xml.Header))
			res.Write(output)
		} else {
			panic(err)
		}
	}
}

func formatRssDesc(sr searchResult) string {
	format := `<i>Age</i>: %s<br /><i>Size</i>: %s<br /><i>Parts</i>: %s<br /><i>Files</i>: %s<br /><i>Subject</i>: %s`
	return fmt.Sprintf(format, sr.Age, sr.Size, sr.Completion, sr.ExtTypes, sr.Subject)
}
