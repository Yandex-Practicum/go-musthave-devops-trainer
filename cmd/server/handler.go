package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"go-musthave-devops-trainer/models"
)

func (s *serverStorage) updateHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	ctx := r.Context()

	// Сервер не санитайзит полученные данные.
	// Вероятно добавим позднее, т.к. боюсь перегружать инкремент.
	var req models.Metrics
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil || req.ID == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Bad request body given"))
		return
	}

	s.Lock()
	defer s.Unlock()
	switch {
	case req.MType == models.Counter && req.Delta != nil:
		data := fmt.Sprintf("%s:%s:%d", req.ID, req.MType, *req.Delta)
		if !s.hashCorrect(data, req.Hash) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("Incorrect hash of counter"))
			return
		}
		count := s.db.UpdateCounter(ctx, req.ID, *req.Delta)
		log.Printf("server: update %s %s=%d, %d\n", req.MType, req.ID, *req.Delta, count)
	case req.MType == models.Gauge && req.Value != nil:
		data := fmt.Sprintf("%s:%s:%f", req.ID, req.MType, *req.Value)
		if !s.hashCorrect(data, req.Hash) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("Incorrect hash of gauge"))
			return
		}
		count := s.db.UpdateGauge(ctx, req.ID, *req.Value)
		log.Printf("server: update %s %s=%.3f, %d\n", req.MType, req.ID, *req.Value, count)
	default:
		w.WriteHeader(http.StatusNotImplemented)
		_, _ = w.Write([]byte("Unknown type of metrics"))
		return
	}
	// w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Content-Encoding", "gzip")
}

func (s *serverStorage) updatesHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	ctx := r.Context()

	var metrics []models.Metrics
	err := json.NewDecoder(r.Body).Decode(&metrics)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Bad request body given"))
		return
	}

	if len(metrics) == 0 {
		w.WriteHeader(http.StatusNoContent)
		_, _ = w.Write([]byte("Empty request"))
	}

	errs := []string{}

	s.Lock()
	defer s.Unlock()
	for _, m := range metrics {
		if m.ID == "" {
			errs = append(errs, "Taked metric with empty ID")
			continue
		}
		switch {
		case m.MType == models.Counter && m.Delta != nil:
			data := fmt.Sprintf("%s:%s:%d", m.ID, m.MType, *m.Delta)
			if !s.hashCorrect(data, m.Hash) {
				errs = append(errs, fmt.Sprintf("Incorrect hash of counter: %q", m.ID))
				continue
			}
			count := s.db.UpdateCounter(ctx, m.ID, *m.Delta)
			log.Printf("server: update %s %s=%d, %d\n", m.MType, m.ID, *m.Delta, count)
		case m.MType == models.Gauge && m.Value != nil:
			data := fmt.Sprintf("%s:%s:%f", m.ID, m.MType, *m.Value)
			if !s.hashCorrect(data, m.Hash) {
				errs = append(errs, fmt.Sprintf("Incorrect hash of gauge: %q", m.ID))
				continue
			}
			count := s.db.UpdateGauge(ctx, m.ID, *m.Value)
			log.Printf("server: update %s %s=%.3f, %d\n", m.MType, m.ID, *m.Value, count)
		default:
			errs = append(errs, fmt.Sprintf("Unknown type %q or content of metrics: %q", m.MType, m.ID))
			continue
		}
	}

	if len(errs) != 0 {
		if len(errs) == len(metrics) {
			w.WriteHeader(http.StatusBadRequest)
		} else {
			w.WriteHeader(http.StatusPartialContent)
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Encoding", "gzip")
		resp := strings.Join(errs, "\n")
		log.Println(resp)
		_, _ = w.Write([]byte(resp))
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *serverStorage) valueHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	ctx := r.Context()

	// Сервер не санитайзит полученные данные.
	// Вероятно добавим позднее, т.к. боюсь перегружать инкремент.
	var m models.Metrics
	err := json.NewDecoder(r.Body).Decode(&m)
	if err != nil || m.ID == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Bad request body given"))
		return
	}
	log.Printf("get %s: %s\n", m.MType, m.ID)

	var ok bool
	s.Lock()
	defer s.Unlock()
	var data string
	switch {
	case m.MType == models.Counter:
		var result int64
		result, ok = s.db.Counter(ctx, m.ID)
		m.Delta = &result
		data = fmt.Sprintf("%s:%s:%d", m.ID, m.MType, *m.Delta)
	case m.MType == models.Gauge:
		var result float64
		result, ok = s.db.Gauge(ctx, m.ID)
		m.Value = &result
		data = fmt.Sprintf("%s:%s:%f", m.ID, m.MType, *m.Value)
	default:
		log.Printf("unknown type of metrics: %s\n", m.MType)
		w.WriteHeader(http.StatusNotImplemented)
		_, _ = w.Write([]byte("Unknown type of metrics"))
		return
	}

	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("Metrics not found"))
		return
	}

	if len(s.key) > 0 {
		h := hmac.New(sha256.New, s.key)
		h.Write([]byte(data))
		hash := fmt.Sprintf("%x", h.Sum(nil))
		m.Hash = hash
	}

	jsonBody, err := json.Marshal(m)
	log.Printf("get result %s: %s, body: %s\n", m.MType, m.ID, jsonBody)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Encoding error"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(jsonBody))
}

func (s *serverStorage) infoHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	ctx := r.Context()

	// Тут бы пригодились шаблоны, но увы...
	// Кроме того, верстку можно было бы сделать лучше,
	// но как мне кажется, это на текущий момент не так уж принципиально.
	// Впрочем, обсуждаемо...
	// w.Header().Set("Content-Type", "text/plain")
	// w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	w.WriteHeader(http.StatusOK)

	s.Lock()
	defer s.Unlock()
	_, _ = io.WriteString(w, `<html>
<head>
<title>Metrics, MustHave.DevOps by Yandex-Practicum</title>
<meta http-equiv="refresh" content="5" />
</head>
<body><h1>Metrics values</h1><h3>Main</h3>`)
	_, _ = io.WriteString(w, `Gen: `+fmt.Sprintf("%d", s.db.UpdateCount(ctx))+"<br>\n")
	_, _ = io.WriteString(w, `Timestamp: `+s.db.Timestamp(ctx, time.StampMilli)+"<br>\n")
	_, _ = io.WriteString(w, `<h3>Counters</h3>`)
	s.db.MapOrderedCounter(ctx, func(k string, v int64) {
		_, _ = io.WriteString(w, k+": "+fmt.Sprintf("%d", v)+"<br>\n")
	})
	_, _ = io.WriteString(w, `<h3>Gauges</h3>`)
	s.db.MapOrderedGauge(ctx, func(k string, v float64) {
		_, _ = io.WriteString(w, k+": "+fmt.Sprintf("%.3f", v)+"<br>\n")
	})
	_, _ = io.WriteString(w, `<html></body></html>`)
}

func (s *serverStorage) hashCorrect(data, hash string) bool {
	if len(s.key) == 0 {
		return true
	}
	h := hmac.New(sha256.New, s.key)
	h.Write([]byte(data))
	return fmt.Sprintf("%x", h.Sum(nil)) == hash
}

func (s *serverStorage) pingHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("ping request")
	if err := s.db.Ping(r.Context()); err != nil {
		log.Printf("ping result: %v\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Println("ping response ok")
}
