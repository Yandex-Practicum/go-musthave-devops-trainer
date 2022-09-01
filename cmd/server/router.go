package main

import (
	"net/http"
	"strings"
	"sync"

	"go-musthave-devops-trainer/internal/store"

	"github.com/go-chi/chi/v5"
)

type serverStorage struct {
	sync.Mutex
	db  store.Store
	key []byte
}

func newRouter(server *serverStorage) http.Handler {
	r := chi.NewRouter()

	r.Use(gzipMiddleware)

	r.Post("/updates/", server.updatesHandler)
	r.Post("/update/", server.updateHandler)
	r.Post("/value/", server.valueHandler)

	r.Post("/update/{type}/{id}/{value}", server.updateHandlerLegacy)
	r.Get("/value/{type}/{id}", server.valueHandlerLegacy)

	r.Get("/", server.infoHandler)

	r.Get("/ping", server.pingHandler)

	return r
}

func gzipMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ow := w

		acceptEncoding := r.Header.Get("Accept-Encoding")
		supportsGzip := strings.Contains(acceptEncoding, "gzip")
		if supportsGzip {
			cw := newCompressWriter(w)
			ow = cw
			defer cw.Close()
		}

		contentEncoding := r.Header.Get("Content-Encoding")
		sendsGzip := strings.Contains(contentEncoding, "gzip")
		if sendsGzip {
			cr, err := newCompressReader(r.Body)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			r.Body = cr
			defer cr.Close()
		}

		h.ServeHTTP(ow, r)
	})
}
