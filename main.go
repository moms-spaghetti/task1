package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	logger := newLogger()
	storage := newStorage(logger)
	server := newServer()
	stop, stopCh := shutdownCh()
	defer stopCh()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			logger.print("GET request")
			key := r.URL.Query().Get("id")

			value, err := storage.get(key)
			if err != nil {
				w.WriteHeader(http.StatusNotFound)

				return
			}

			data, _ := json.MarshalIndent(
				struct {
					ID   string
					Data interface{}
				}{
					ID:   key,
					Data: value,
				},
				"",
				"\t",
			)
			if _, err := w.Write(data); err != nil {
				logger.print("GET error writing response %v", err)
			}

		case "POST":
			logger.print("POST request")
			body, _ := io.ReadAll(r.Body)

			var d map[string]interface{}

			if err := json.Unmarshal(body, &d); err != nil {
				logger.print("POST error unmarshalling response %v", err)
			}

			storage.save(d)

		default:
			logger.print("unknown route")
			w.WriteHeader(http.StatusBadRequest)
		}
	})

	storage.listener()
	logger.listener()
	go start(server)

	<-stop

	shutdown(context.Background(), server)

}

// server

func newServer() *http.Server {
	return &http.Server{
		Addr:    ":9000",
		Handler: nil,
	}
}

func start(server *http.Server) {
	if err := server.ListenAndServe(); err != nil {
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}
	log.Print("server graceful shutdown")
}

func shutdownCh() (chan os.Signal, func()) {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)

	return stop, func() {
		close(stop)
	}
}

func shutdown(ctx context.Context, server *http.Server) {
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := server.Shutdown(cctx); err != nil {
		log.Fatal(err)
	}

	log.Print("shutdown complete")
}

// storage

type storage struct {
	storeC chan map[string]interface{}
	store  map[string]interface{}
	logger logger
}

func newStorage(l logger) storage {
	log.Print("new storage created")

	return storage{
		storeC: make(chan map[string]interface{}),
		store:  map[string]interface{}{},
		logger: l,
	}
}

func (s *storage) listener() {
	go func() {
		for {
			for k, v := range <-s.storeC {
				s.store[k] = v
			}
		}
	}()
}

func (s *storage) save(data map[string]interface{}) {
	s.logger.print("saved data: %v", data)

	s.storeC <- data
}

func (s storage) get(k string) (interface{}, error) {
	if k == "" {
		s.logger.print("key empty")

		return nil, errors.New("key empty")
	}

	v, ok := s.store[k]
	if !ok {
		s.logger.print("no value")

		return nil, errors.New("no value")
	}

	s.logger.print(k, v)
	return v, nil
}

// logger

type logger struct {
	logC       chan string
	timeFormat string
}

func newLogger() logger {
	log.Print("new logger created")

	return logger{
		logC:       make(chan string),
		timeFormat: time.RFC3339,
	}
}

func (l logger) print(message ...interface{}) {
	t := time.Now().Format(l.timeFormat)
	l.logC <- fmt.Sprintf("logger: %s : %v", t, message)
}

func (l *logger) listener() {
	go func() {

		for {
			fmt.Println(<-l.logC)
		}
	}()
}
