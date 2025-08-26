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
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"net/http/pprof"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/crypto/acme/autocert"
	"gopkg.in/yaml.v2"
)

//go:embed defaults
var defaultPages embed.FS

// --- Global Application State ---
var app = &App{}

// --- Metrics Definitions ---
var (
	httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "clara_http_requests_total",
		Help: "Total number of HTTP requests.",
	}, []string{"service", "path", "code"})

	httpRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "clara_http_request_duration_seconds",
		Help:    "Duration of HTTP requests.",
		Buckets: prometheus.DefBuckets,
	}, []string{"service", "path"})
)

// Service defines a backend service, supporting either a single host or multiple servers for load balancing.
type Service struct {
	Name                  string   `yaml:"name"`
	Host                  string   `yaml:"host,omitempty"`
	Port                  int      `yaml:"port,omitempty"`
	LoadBalancingStrategy string   `yaml:"load_balancing_strategy,omitempty"`
	Servers               []string `yaml:"servers,omitempty"`
}

// LoadBalancer holds the logic for a round-robin setup.
type LoadBalancer struct {
	backends []*httputil.ReverseProxy
	mu       sync.Mutex
	next     int
}

// ServeHTTP for LoadBalancer makes it a valid http.Handler, distributing requests to backends.
func (lb *LoadBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	lb.mu.Lock()
	backend := lb.backends[lb.next%len(lb.backends)]
	lb.next++
	lb.mu.Unlock()
	backend.ServeHTTP(w, r)
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

// routeHandler holds a path and its corresponding handler, which can be a single proxy or a load balancer.
type routeHandler struct {
	path    string
	service string
	handler http.Handler
}

// Custom responseWriter to get the status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// NewResponseWriter creates a new responseWriter.
func NewResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{w, http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// metricsMiddleware wraps an http.Handler to record Prometheus metrics.
func metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		res := NewResponseWriter(w)
		var router *Router
		if rtr, ok := app.router.Load().(*Router); ok {
			router = rtr
		}

		routePath := "unmatched"
		serviceName := "unmatched"
		if router != nil {
			match := router.findBestMatch(r.URL.Path)
			if match != nil {
				routePath = match.path
				serviceName = match.service
			}
		}

		startTime := time.Now()
		next.ServeHTTP(res, r)
		duration := time.Since(startTime)

		httpRequestDuration.WithLabelValues(serviceName, routePath).Observe(duration.Seconds())
		httpRequestsTotal.WithLabelValues(serviceName, routePath, strconv.Itoa(res.statusCode)).Inc()
	})
}

// serveErrorPage handles serving custom or embedded error pages.
func (r *Router) serveErrorPage(w http.ResponseWriter, req *http.Request, statusCode int) {
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

	defaultPagePath := fmt.Sprintf("defaults/%d.html", statusCode)
	htmlBytes, err := defaultPages.ReadFile(defaultPagePath)
	if err == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(statusCode)
		w.Write(htmlBytes)
		return
	}

	http.Error(w, http.StatusText(statusCode), statusCode)
}

// findBestMatch extracts the route matching logic.
func (r *Router) findBestMatch(requestPath string) *routeHandler {
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
	return bestMatch
}

// ServeHTTP implements the http.Handler interface with custom routing logic.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if len(r.routes) == 0 && req.URL.Path == "/" {
		htmlBytes, err := defaultPages.ReadFile("defaults/welcome.html")
		if err == nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			w.Write(htmlBytes)
			return
		}
	}

	bestMatch := r.findBestMatch(req.URL.Path)

	if bestMatch != nil {
		log.Printf("Clara received request for '%s', proxying to service '%s' (match: '%s')", req.URL.Path, bestMatch.service, bestMatch.path)
		bestMatch.handler.ServeHTTP(w, req)
		return
	}

	log.Printf("Clara received request for '%s' - no matching route found, returning 404", req.URL.Path)
	r.serveErrorPage(w, req, http.StatusNotFound)
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

		var handler http.Handler

		if len(svc.Servers) > 0 { // This service uses a load balancer
			lb := &LoadBalancer{}
			for _, serverURL := range svc.Servers {
				target, err := url.Parse(serverURL)
				if err != nil {
					log.Printf("Warning: Failed to parse target URL '%s' for service '%s': %v", serverURL, svc.Name, err)
					continue
				}
				proxy := httputil.NewSingleHostReverseProxy(target)
				originalDirector := proxy.Director
				proxy.Director = func(req *http.Request) {
					originalDirector(req)
					req.URL.Path = strings.TrimPrefix(req.URL.Path, route.Path)
					req.RequestURI = ""
				}
				lb.backends = append(lb.backends, proxy)
			}
			if len(lb.backends) > 0 {
				handler = lb
				log.Printf("Initialized round-robin load balancer for service '%s' with %d servers.", svc.Name, len(lb.backends))
			}
		} else if svc.Host != "" { // This service uses a single backend
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
			handler = proxy
		}

		if handler != nil {
			router.routes = append(router.routes, routeHandler{
				path:    route.Path,
				service: route.Service,
				handler: handler,
			})
		}
	}

	return router
}

func main() {
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

	go func() { // Hot reloading goroutine
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

	go func() { // Metrics server goroutine
		metricsMux := http.NewServeMux()
		metricsMux.Handle("/metrics", promhttp.Handler())
		metricsMux.HandleFunc("/debug/pprof/", pprof.Index)
		log.Println("Starting metrics server on :9091")
		if err := http.ListenAndServe(":9091", metricsMux); err != nil {
			log.Fatalf("Metrics server failed: %v", err)
		}
	}()

	mainHandler := func(w http.ResponseWriter, r *http.Request) {
		if router, ok := app.router.Load().(*Router); ok {
			router.ServeHTTP(w, r)
		} else {
			http.Error(w, "Service unavailable", http.StatusInternalServerError)
		}
	}

	wrappedHandler := metricsMiddleware(http.HandlerFunc(mainHandler))

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
			Handler:   wrappedHandler,
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
			Handler: wrappedHandler,
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
