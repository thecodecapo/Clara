package main

import (
    "fmt"
    "log"
    "net/http"
)

func main() {
    // This handler will respond with "Hello from the backend!"
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        log.Println("Received request on backend server.")
        fmt.Fprint(w, "Hello from the backend!")
    })

    // Start the backend on port 3000, as defined in your config.yaml
    log.Println("Starting backend server on :3000")
    if err := http.ListenAndServe(":3000", nil); err != nil {
        log.Fatalf("Failed to start backend: %v", err)
    }
}