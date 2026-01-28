package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {
	fmt.Println("🛌 Doze API Server")
	fmt.Println("Starting on :8080...")

	http.HandleFunc("/status", handleStatus)
	http.HandleFunc("/start", handleStart)
	http.HandleFunc("/stream", handleStream)
	http.HandleFunc("/message", handleMessage)

	log.Fatal(http.ListenAndServe(":8080", nil))
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
