# Clara: The Clear & Declarative Reverse Proxy

Clara is a modern, open-source reverse proxy built in Go, designed for simplicity, clarity, and performance. Unlike traditional proxies that rely on complex, imperative scripts, Clara uses a simple declarative YAML file to manage your routing.

Just declare the state you want, and Clara handles the rest.

## ✨ Features


*   **Declarative YAML Configuration:** No complex syntax. Just a clean, human-readable file.
    
*   **Automatic HTTPS:** Powered by Let's Encrypt, Clara automatically provisions and renews TLS certificates.
    
*   **Hot Reloading:** Change your configuration and Clara reloads it on the fly with zero downtime. No restarts needed.
    
*   **High Performance:** Built in Go to be fast, lightweight, and highly concurrent.
    
*   **Graceful Shutdown:** Protects against dropped connections during restarts or deployments.
    
*   **Path Stripping:** Intelligently forwards requests to backend services without path prefixes.
    

🚀 Getting Started
------------------

### Prerequisites

*   **Go 1.22** or newer.
    
*   A publicly accessible server (like an AWS EC2 instance) with a public IP.
    
*   A domain name pointed to your server's public IP.
    
*   Ports **80** and **443** open on your server's firewall.
    

### Installation & Usage

1.  Bashgit clone https://github.com/thecodecapo/Clara.gitcd Clara
    
2.  **Create your configuration file:**Create a file named config.yaml in the same directory.
    
3.  Bashsudo go run main.goClara will now start, read your configuration, and begin serving traffic.
    

⚙️ Configuration
----------------

Clara is configured using a single config.yaml file. Here is a complete example:

YAML

 ```
   # (Optional) Enable automatic HTTPS with Let's Encrypt.
# If this section is present, Clara will run in HTTPS mode.
tls:
  # Email for Let's Encrypt account notices.
  email: "your-email@example.com"
  # List of domains to secure. Must be pointed to this server.
  domains:
    - "api.yourdomain.com"
    - "www.yourdomain.com"

# Define your backend services.
services:
  - name: "my-api-service"
    host: "localhost"
    port: 4000
    
  - name: "my-website"
    host: "localhost"
    port: 3000

# Define the routes that map incoming paths to your services.
routes:
  # Requests to "api.yourdomain.com/" will be proxied to the "my-api-service".
  - path: "/"
    service: "my-api-service"
    
  # Requests starting with "/blog/" will be proxied to the "my-website" service.
  # The "/blog/" prefix will be stripped before forwarding.
  - path: "/blog/"
    service: "my-website"  
   ```

🛣️ Project Roadmap
-------------------

Clara is actively developed. Here's what's planned for the future:

*   \[ \] **Load Balancing:** Strategies like round-robin to distribute traffic across multiple service instances.
    
*   \[ \] **Metrics & Observability:** A /metrics endpoint for Prometheus integration.
    
*   \[ \] **Websocket Support:** Full support for proxying websocket connections.
    
*   \[ \] **Plugin System:** Extend Clara's functionality with custom middleware.
    

🙌 Contributing
---------------

We welcome contributions of all kinds! If you're interested in helping, please:

1.  Fork the repository.
    
2.  Create a new branch for your feature or bugfix.
    
3.  Open a pull request with a clear description of your changes.
    

If you find a bug or have a feature request, please [open an issue](https://github.com/thecodecapo/Clara/issues).

Go Source Code (main.go)
------------------------

```
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/crypto/acme/autocert"
	"gopkg.in/yaml.v2"
)

// Service defines a backend service.
type Service struct {
	Name string `yaml:"name"`
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

// Route defines how to handle incoming requests.
type Route struct {
	Path    string `yaml:"path"`
	Service string `yaml:"service"`
}

// TLS holds the configuration for automatic HTTPS.
type TLS struct {
	Email   string   `yaml:"email"`
	Domains []string `yaml:"domains"`
}

// Config represents the structure of your config.yaml file.
type Config struct {
	TLS      *TLS      `yaml:"tls"`
	Services []Service `yaml:"services"`
	Routes   []Route   `yaml:"routes"`
}

// App holds the current application state, including the router.
type App struct {
	router atomic.Value
}

// Router represents our dynamic routing table.
type Router struct {
	routes []routeHandler
}

type routeHandler struct {
	path    string
	service string
	proxy   *httputil.ReverseProxy
}

// ServeHTTP implements the http.Handler interface with custom routing logic.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	requestPath := req.URL.Path

	var bestMatch *routeHandler
	bestMatchLen := -1

	for i := range r.routes {
		route := &r.routes[i]

		// Handle exact matches first.
		if route.path == requestPath {
			bestMatch = route
			break
		}

		// Handle prefix matches.
		if route.path != "/" && strings.HasSuffix(route.path, "/") {
			if strings.HasPrefix(requestPath, route.path) {
				if len(route.path) > bestMatchLen {
					bestMatch = route
					bestMatchLen = len(route.path)
				}
			}
		}
	}

	if bestMatch != nil {
		log.Printf("Clara received request for '%s', proxying to service '%s' (match: '%s')", requestPath, bestMatch.service, bestMatch.path)
		bestMatch.proxy.ServeHTTP(w, req)
		return
	}

	log.Printf("Clara received request for '%s' - no matching route found, returning 404", requestPath)
	http.NotFound(w, req)
}

func (a *App) newRouter(config *Config) *Router {
	router := &Router{
		routes: make([]routeHandler, 0),
	}
	serviceMap := make(map[string]Service)

	for _, svc := range config.Services {
		serviceMap[svc.Name] = svc
	}

	for _, route := range config.Routes {
		svc, exists := serviceMap[route.Service]
		if !exists {
			log.Printf("Warning: Route for path '%s' references a service '%s' that does not exist.", route.Path, route.Service)
			continue
		}

		targetURL, err := url.Parse(fmt.Sprintf("http://%s:%d", svc.Host, svc.Port))
		if err != nil {
			log.Printf("Warning: Failed to parse target URL for service '%s': %v", svc.Name, err)
			continue
		}

		proxy := httputil.NewSingleHostReverseProxy(targetURL)
		originalDirector := proxy.Director

		proxy.Director = func(req *http.Request) {
			originalDirector(req)
			req.URL.Path = strings.TrimPrefix(req.URL.Path, route.Path)
			req.RequestURI = ""
		}

		router.routes = append(router.routes, routeHandler{
			path:    route.Path,
			service: route.Service,
			proxy:   proxy,
		})
	}

	return router
}

func main() {
	app := &App{}
	var config Config

	loadAndServeConfig := func() error {
		data, err := os.ReadFile("config.yaml")
		if err != nil {
			return fmt.Errorf("error reading config file: %w", err)
		}
		if err := yaml.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("error parsing config file: %w", err)
		}
		app.router.Store(app.newRouter(&config))
		return nil
	}

	if err := loadAndServeConfig(); err != nil {
		log.Fatalf("Initial config load failed: %v", err)
	}

	go func() {
		lastModTime, _ := os.Stat("config.yaml")
		for {
			time.Sleep(3 * time.Second)
			stat, err := os.Stat("config.yaml")
			if err != nil {
				log.Printf("Error stating config file: %v", err)
				continue
			}
			if stat.ModTime() != lastModTime.ModTime() {
				log.Println("Change detected in config.yaml, reloading...")
				if err := loadAndServeConfig(); err != nil {
					log.Printf("Config reload failed: %v", err)
				} else {
					log.Println("Clara has reloaded the configuration successfully.")
				}
				lastModTime = stat
			}
		}
	}()

	mainHandler := func(w http.ResponseWriter, r *http.Request) {
		if router, ok := app.router.Load().(*Router); ok {
			router.ServeHTTP(w, r)
		} else {
			http.Error(w, "Service unavailable", http.StatusInternalServerError)
		}
	}

	var server *http.Server
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	if config.TLS != nil && len(config.TLS.Domains) > 0 {
		log.Println("TLS is configured. Setting up Automatic HTTPS...")

		certManager := &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(config.TLS.Domains...),
			Cache:      autocert.DirCache("certs"),
			Email:      config.TLS.Email,
		}

		server = &http.Server{
			Addr:      ":443",
			Handler:   mainHandler,
			TLSConfig: certManager.TLSConfig(),
		}

		go func() {
			log.Println("Starting HTTP server on :80 for ACME challenges and redirects.")
			if err := http.ListenAndServe(":80", certManager.HTTPHandler(nil)); err != nil {
				log.Printf("HTTP server for ACME challenges failed: %v", err)
			}
		}()

		go func() {
			log.Println("Clara is ready. Starting HTTPS server on :443")
			if err := server.ListenAndServeTLS("", ""); err != http.ErrServerClosed {
				log.Fatalf("HTTPS Server ListenAndServeTLS: %v", err)
			}
		}()
	} else {
		log.Println("Clara is ready. Starting HTTP server on :8080")
		server = &http.Server{
			Addr:    ":8080",
			Handler: mainHandler,
		}

		go func() {
			if err := server.ListenAndServe(); err != http.ErrServerClosed {
				log.Fatalf("HTTP Server ListenAndServe: %v", err)
			}
		}()
	}

	<-stop
	log.Println("Shutting down Clara...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Graceful shutdown failed: %v", err)
	}

	log.Println("Clara has gracefully shut down.")
}

```