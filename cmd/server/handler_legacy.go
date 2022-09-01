package main

import (
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

func (s *serverStorage) updateHandlerLegacy(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	ctx := r.Context()

	// Сервер не санитайзит полученные данные.
	id := chi.URLParam(r, "id")

	if id == "" {
		http.Error(w, "undefined field 'id'", http.StatusBadRequest)
		return
	}

	rawValue := chi.URLParam(r, "value")
	if rawValue == "" {
		http.Error(w, "undefined field 'value'", http.StatusBadRequest)
		return
	}

	var count int
	reqType := chi.URLParam(r, "type")

	s.Lock()
	defer s.Unlock()
	switch reqType {
	case "counter":
		delta, err := strconv.ParseInt(rawValue, 10, 64)
		if err != nil {
			http.Error(w, "wrong type of counter value", http.StatusBadRequest)
			return
		}
		count = s.db.UpdateCounter(ctx, id, delta)
	case "gauge":
		value, err := strconv.ParseFloat(rawValue, 64)
		if err != nil {
			http.Error(w, "wrong type of gauge value", http.StatusBadRequest)
			return
		}
		count = s.db.UpdateGauge(ctx, id, value)
	default:
		http.Error(w, "unknown type of metrics", http.StatusNotImplemented)
		return
	}

	log.Printf("update %s: %s=%s, %d\n", reqType, id, rawValue, count)
	_, _ = w.Write([]byte("Updated: " + fmt.Sprintf("%d\n", count)))
}

func (s *serverStorage) valueHandlerLegacy(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	ctx := r.Context()

	// Сервер не санитайзит полученные данные.
	id := chi.URLParam(r, "id")

	if id == "" {
		http.Error(w, "undefined field 'id'", http.StatusBadRequest)
		return
	}

	reqType := chi.URLParam(r, "type")

	s.Lock()
	defer s.Unlock()
	switch reqType {
	case "counter":
		if v, ok := s.db.Counter(ctx, id); ok {
			_, _ = w.Write([]byte(fmt.Sprintf("%d", v)))
			return
		}
	case "gauge":
		if v, ok := s.db.Gauge(ctx, id); ok {
			_, _ = w.Write([]byte(fmt.Sprintf("%.3f", v)))
			return
		}
	default:
		http.Error(w, "unknown type of metrics", http.StatusNotImplemented)
		return
	}
	http.NotFound(w, r)
}
