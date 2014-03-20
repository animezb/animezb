package main

import (
	"flag"
	"fmt"
	"github.com/codegangsta/martini"
	"github.com/martini-contrib/gzip"
	"log"
	//"net"
	"github.com/animezb/goes"
	"github.com/codegangsta/inject"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var httpport int
var htmldir string
var elasticsearch string
var useGzip bool

type HasBytes interface {
	Bytes() []byte
}

func initMartini() (*martini.ClassicMartini, error) {
	m := martini.Classic()
	if useGzip {
		m.Use(gzip.All())
	}
	asJson := func(res http.ResponseWriter) {
		res.Header().Set("Content-Type", "application/json")
	}
	isBytes := func(v reflect.Value) bool {
		return v.Kind() == reflect.Slice && v.Type().Elem().Kind() == reflect.Uint8
	}
	returnHandler := func() martini.ReturnHandler {
		return func(ctx martini.Context, vals []reflect.Value) {
			rv := ctx.Get(inject.InterfaceOf((*http.ResponseWriter)(nil)))
			res := rv.Interface().(http.ResponseWriter)
			var val reflect.Value
			if len(vals) > 1 && vals[0].Kind() == reflect.Int {
				if vals[0].Int() == 0 {
					return
				}
			}
			if len(vals) > 1 && vals[0].Kind() == reflect.Int {
				res.WriteHeader(int(vals[0].Int()))
				val = vals[1]
			} else if len(vals) == 1 && vals[0].Kind() == reflect.Int {
				res.WriteHeader(int(vals[0].Int()))
				return
			} else if len(vals) > 0 {
				val = vals[0]
			}
			if !val.IsValid() {
				return
			}
			if val.Kind() == reflect.Interface || val.Kind() == reflect.Ptr {
				val = val.Elem()
			}
			if !val.IsValid() {
				return
			}
			if isBytes(val) {
				res.Write([]byte(val.Bytes()))
			} else if val.Kind() == reflect.String {
				res.Write([]byte(val.String()))
			} else if v, ok := val.Interface().(interface {
				Bytes() []byte
			}); ok {
				res.Write(v.Bytes())
			} else {
				res.Write([]byte(val.String()))
			}
		}
	}
	origin := func(req *http.Request, res http.ResponseWriter) {
		origin := req.Header.Get("Origin")
		custHead := req.Header.Get("Access-Control-Request-Headers")
		if req.Method != "OPTIONS" && (strings.Contains(strings.ToLower(origin), "animezb.com")) {
			res.Header().Set("Access-Control-Allow-Origin", origin)
			res.Header().Set("Access-Control-Allow-Methods", "GET,PUT,POST,DELETE,PATCH")
			if custHead != "" {
				res.Header().Set("Access-Control-Allow-Headers", custHead)
			} else {
				res.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			}
			res.Header().Set("Access-Control-Expose-Headers", "Content-Type")
			res.Header().Set("Access-Control-Allow-Credentials", "true")
		}
	}

	m.Use(origin)
	m.Map(martini.ReturnHandler(returnHandler()))

	m.Options("/***", func(res http.ResponseWriter, req *http.Request) (int, string) {
		origin := req.Header.Get("Origin")
		custHead := req.Header.Get("Access-Control-Request-Headers")
		if strings.Contains(strings.ToLower(origin), "animezb.com") {
			res.Header().Set("Access-Control-Allow-Origin", origin)
			res.Header().Set("Access-Control-Allow-Methods", "GET,PUT,POST,DELETE,PATCH")
			if custHead != "" {
				res.Header().Set("Access-Control-Allow-Headers", custHead)
			} else {
				res.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			}
			res.Header().Set("Access-Control-Expose-Headers", "Content-Type, Results-Count, Query-Skip, Query-Limit")
			res.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		return 204, ""
	})

	eshost := elasticsearch
	esport := 9200
	if strings.Contains(eshost, ":") {
		port := eshost[strings.LastIndex(eshost, ":"):]
		eshost = eshost[:strings.LastIndex(eshost, ":")]
		if n, err := strconv.Atoi(port); err == nil {
			esport = n
		}
	}

	ctx := &context{
		EsConn:  goes.NewConnection(eshost, esport),
		EsHost:  eshost,
		EsPort:  esport,
		HtmlDir: http.Dir(htmldir),
	}

	m.Map(ctx)

	routes(m)

	m.NotFound(asJson, func() (int, string) {
		return 404, "{\"error\":\"404\"}"
	})

	return m, nil
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	flag.IntVar(&httpport, "p", 2333, "Server Port")
	flag.BoolVar(&useGzip, "gz", false, "Gzip Compression")
	flag.StringVar(&htmldir, "d", "./www", "Server html directory.")
	flag.StringVar(&elasticsearch, "es", "localhost:9200", "ElasticSearch server host & port.")
	flag.Parse()

	log.Print("Starting http server...")

	if m, e := initMartini(); e == nil {
		go func() {
			s := &http.Server{
				Addr:           fmt.Sprintf(":%d", httpport),
				Handler:        m,
				ReadTimeout:    5 * time.Minute,
				WriteTimeout:   5 * time.Minute,
				MaxHeaderBytes: 1 << 20,
			}
			log.Fatal(s.ListenAndServe())
		}()
	} else {
		log.Fatal("Failed to start web server.", e)
	}

	exitChannel := make(chan bool)
	exitFunc := func() {
		exitChannel <- true
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		forceExit := false
		for _ = range c {
			if forceExit {
				os.Exit(2)
			} else {
				go func() {
					exitFunc()
				}()
				forceExit = true
			}
		}
	}()

	<-exitChannel
	log.Println("Bye")

}
