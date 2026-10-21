package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/Personal-AI-Architecture/go-architecture-template/internal/adapters"
	"github.com/Personal-AI-Architecture/go-architecture-template/internal/engine"
	"github.com/Personal-AI-Architecture/go-architecture-template/internal/gateway"
)

func main() {
	host := flag.String("host", "127.0.0.1", "HTTP bind host")
	port := flag.Int("port", 8000, "HTTP bind port")
	flag.Parse()

	service := gateway.New(engine.NewAgentLoop(adapters.MockAdapter{}, nil), nil)
	address := fmt.Sprintf("%s:%d", *host, *port)
	fmt.Printf("PAA Go template listening at http://%s\n", address)
	fmt.Printf("Try: curl http://%s/health\n", address)
	fmt.Println("Press Ctrl-C to stop.")
	log.Fatal(http.ListenAndServe(address, gateway.Handler(service)))
}
