package main

import (
    "fmt"
    "log"
    "net/http"
)

func main() {
    // This handler will respond with "Hello from the API backend!"
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        log.Println("Received request on API backend server.")
        fmt.Fprint(w, "Hello from the API backend!")
    })

    // Start the backend on port 4000, as defined in your config.yaml
    log.Println("Starting API backend server on :4000")
    if err := http.ListenAndServe(":4000", nil); err != nil {
        log.Fatalf("Failed to start backend: %v", err)
    }
}