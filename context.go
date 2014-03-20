package main

import (
	"github.com/animezb/goes"
	"net/http"
)

type context struct {
	EsConn  *goes.Connection
	HtmlDir http.Dir
	EsHost  string
	EsPort  int
}
