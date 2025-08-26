package main

import (
	"context"
	"embed"
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

//go:embed defaults
var defaultPages embed.FS

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
	ErrorPages map[int]string `yaml:"error_pages"` 
	TLS        *TLS           `yaml:"tls"`
	Services   []Service      `yaml:"services"`
	Routes     []Route        `yaml:"routes"`
}

// App holds the current application state.
type App struct {
	router atomic.Value
}

// Router represents our dynamic routing table.
type Router struct {
	routes     []routeHandler
	errorPages map[int]string
}

type routeHandler struct {
	path    string
	service string
	proxy   *httputil.ReverseProxy
}

// serveErrorPage handles serving custom or embedded error pages.
func (r *Router) serveErrorPage(w http.ResponseWriter, req *http.Request, statusCode int) {
	// First, check for a user-defined custom error page
	if pagePath, exists := r.errorPages[statusCode]; exists {
		htmlBytes, err := os.ReadFile(pagePath)
		if err == nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(statusCode)
			w.Write(htmlBytes)
			return 
		}
		log.Printf("Warning: Failed to read custom error page '%s': %v", pagePath, err)
	}

	// If no custom page, fall back to our embedded default page
	defaultPagePath := fmt.Sprintf("defaults/%d.html", statusCode)
	htmlBytes, err := defaultPages.ReadFile(defaultPagePath)
	if err == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(statusCode)
		w.Write(htmlBytes)
		return 
	}

	// As a final resort, fall back to the plain text error
	http.Error(w, http.StatusText(statusCode), statusCode)
}

// ServeHTTP implements the http.Handler interface with custom routing logic.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	requestPath := req.URL.Path

	var bestMatch *routeHandler
	bestMatchLen := -1

	for i := range r.routes {
		route := &r.routes[i]
		if route.path == requestPath {
			bestMatch = route
			break
		}
		if route.path != "/" && strings.HasSuffix(route.path, "/") {
			if strings.HasPrefix(requestPath, route.path) {
				if len(route.path) > bestMatchLen {
					bestMatch = route
					bestMatchLen = len(route.path)
				}
			}
		}
	}

	if len(r.routes) == 0 && req.URL.Path == "/" {
		htmlBytes, err := defaultPages.ReadFile("defaults/welcome.html")
		if err == nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			w.Write(htmlBytes)
			return
		}
	}

	if bestMatch != nil {
		log.Printf("Clara received request for '%s', proxying to service '%s' (match: '%s')", requestPath, bestMatch.service, bestMatch.path)
		bestMatch.proxy.ServeHTTP(w, req)
		return
	}

	log.Printf("Clara received request for '%s' - no matching route found, returning 404", requestPath)
	r.serveErrorPage(w, req, http.StatusNotFound) // 👈 NOTE: Using the new error handler
}

// newRouter creates a new router instance from the configuration.
func (a *App) newRouter(config *Config) *Router {
	router := &Router{
		routes:     make([]routeHandler, 0),
		errorPages: config.ErrorPages,
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
			Handler:   http.HandlerFunc(mainHandler),
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
			Handler: http.HandlerFunc(mainHandler),
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