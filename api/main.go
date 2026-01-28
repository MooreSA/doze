package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Println("🛌 Doze API Server")
	fmt.Printf("Starting on :%s...\n", port)

	// API endpoints
	http.HandleFunc("/status", handleStatus)
	http.HandleFunc("/start", handleStart)
	http.HandleFunc("/stream", handleStream)
	http.HandleFunc("/message", handleMessage)

	// Serve web UI
	http.HandleFunc("/", handleIndex)

	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	// TODO: implement
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"ok","message":"Doze API - coming soon"}`)
}

func handleStart(w http.ResponseWriter, r *http.Request) {
	// TODO: implement
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"error":"not implemented"}`)
	w.WriteHeader(http.StatusNotImplemented)
}

func handleStream(w http.ResponseWriter, r *http.Request) {
	// TODO: implement SSE streaming
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	fmt.Fprintf(w, "data: {\"type\":\"info\",\"content\":\"Doze streaming - coming soon\"}\n\n")
}

func handleMessage(w http.ResponseWriter, r *http.Request) {
	// TODO: implement
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"error":"not implemented"}`)
	w.WriteHeader(http.StatusNotImplemented)
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	// Serve web UI from /web directory
	webPath := os.Getenv("WEB_PATH")
	if webPath == "" {
		webPath = "../web"
	}

	indexPath := filepath.Join(webPath, "index.html")
	http.ServeFile(w, r, indexPath)
}
