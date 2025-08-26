package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	// Get the port from the command-line argument, default to 3000
	port := "3000"
	if len(os.Args) > 1 {
		port = os.Args[1]
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Received request on backend server running on port %s", port)
		fmt.Fprintf(w, "Hello from the backend on port %s!", port)
	})

	log.Printf("Starting dummy backend server on :%s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}
