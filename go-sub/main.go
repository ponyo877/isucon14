package main

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/kaz/pprotein/integration/standalone"
	pb "github.com/ponyo877/isucon14/go-sub/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type Coordinate struct {
	Latitude  int `json:"latitude"`
	Longitude int `json:"longitude"`
}

func main() {
	go func() {
		standalone.Integrate(":19001")
	}()
	mux := setup()
	slog.Info("Listening on :8080")
	go http.ListenAndServe(":8080", mux)

	listener, err := net.Listen("tcp", ":8081")
	if err != nil {
		panic(err)
	}
	s := grpc.NewServer()
	pb.RegisterSubServiceServer(s, NewServer())
	reflection.Register(s)

	slog.Info("Listening on :8081")
	s.Serve(listener)
}

func setup() http.Handler {
	mux := chi.NewRouter()
	mux.Use(middleware.Logger)
	mux.Use(middleware.Recoverer)

	authedAppMux := mux.With(appAuthMiddleware)
	authedAppMux.HandleFunc("GET /api/app/notification", appGetNotification)
	authedChairMux := mux.With(chairAuthMiddleware)
	authedChairMux.HandleFunc("GET /api/chair/notification", chairGetNotification)

	return mux
}

func writeError(w http.ResponseWriter, statusCode int, err error) {
	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	w.WriteHeader(statusCode)
	buf, marshalError := json.Marshal(map[string]string{"message": err.Error()})
	if marshalError != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"marshaling error failed"}`))
		return
	}
	w.Write(buf)
	slog.Error("error response wrote", err)
}
