package main

import (
	"github.com/codegangsta/martini"
)

func routes(m *martini.ClassicMartini) {

	m.Get("/", search)
	m.Get("/index", search)
	m.Get("/index.html", search)

	m.Get("/nzb/:nzbid/:nzbname", gennzb)
	m.Get("/nzb/:nzbid", gennzb)
	m.Post("/nzb", gennzb)

	m.Get("/rss", genrss)
	m.Get("/rss/", genrss)
	m.Get("/uploads/:nzbid", getUploadInfo)
	m.Use(martini.Static("www"))

}
