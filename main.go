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

    "gopkg.in/yaml.v2"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)



type Service struct {
    Name string `yaml:"name"`
    Host string `yaml:"host"`
    Port int    `yaml:"port"`
}


type Route struct {
    Path    string `yaml:"path"`
    Service string `yaml:"service"`
}


type App struct {
    router atomic.Value
}


type Router struct {
    routes []routeHandler
}

type routeHandler struct {
    path    string
    service string
    proxy   *httputil.ReverseProxy
}


type TLS struct {
    Email   string   `yaml:"email"`
    Domains []string `yaml:"domains"`
}


type Config struct {
    TLS      *TLS      `yaml:"tls"`
    Services []Service `yaml:"services"`
    Routes   []Route   `yaml:"routes"`
}


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
    var config Config // Declare config here to access it later

    loadAndServeConfig := func() error {
        data, err := os.ReadFile("config.yaml")
        if err != nil {
            return fmt.Errorf("error reading config file: %w", err)
        }
        // Unmarshal into the config variable in the outer scope
        if err := yaml.Unmarshal(data, &config); err != nil {
            return fmt.Errorf("error parsing config file: %w", err)
        }
        app.router.Store(app.newRouter(&config))
        return nil
    }

    if err := loadAndServeConfig(); err != nil {
        log.Fatalf("Initial config load failed: %v", err)
    }

    // Goroutine for hot reloading (remains the same)
    go func() {
        // ... your existing hot reload logic ...
    }()

    mainHandler := func(w http.ResponseWriter, r *http.Request) {
        if router, ok := app.router.Load().(*Router); ok {
            router.ServeHTTP(w, r)
        } else {
            http.Error(w, "Service unavailable", http.StatusInternalServerError)
        }
    }

    
    // Check if TLS is configured.
    if config.TLS != nil && len(config.TLS.Domains) > 0 {
        log.Println("TLS is configured. Setting up Automatic HTTPS...")

      // Create the autocert manager for production.
certManager := &autocert.Manager{
    Prompt:     autocert.AcceptTOS,
    HostPolicy: autocert.HostWhitelist(config.TLS.Domains...),
    Cache:      autocert.DirCache("certs"),
    Email:      config.TLS.Email,
}
}

        // Create the main HTTPS server.
        httpsServer := &http.Server{
            Addr:    ":443",
            Handler: http.HandlerFunc(mainHandler),
            // GetCertificate is the magic that connects the server with the cert manager.
            TLSConfig: certManager.TLSConfig(),
        }

        // We still need an HTTP server on port 80 to handle ACME challenges
        // and redirect other traffic to HTTPS.
        go func() {
            log.Println("Starting HTTP server on :80 for ACME challenges and redirects.")
            // The handler for this is provided by the certManager.
            if err := http.ListenAndServe(":80", certManager.HTTPHandler(nil)); err != nil {
                log.Printf("HTTP server for ACME challenges failed: %v", err)
            }
        }()

        // Start the main HTTPS server.
        go func() {
            log.Println("Clara is ready. Starting HTTPS server on :443")
            if err := httpsServer.ListenAndServeTLS("", ""); err != http.ErrServerClosed {
                log.Fatalf("HTTPS Server ListenAndServeTLS: %v", err)
            }
        }()

        // Graceful shutdown logic needs to be aware of the HTTPS server now.
        stop := make(chan os.Signal, 1)
        signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
        <-stop

        log.Println("Shutting down Clara...")
        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        defer cancel()
        if err := httpsServer.Shutdown(ctx); err != nil {
            log.Fatalf("Graceful HTTPS shutdown failed: %v", err)
        }

    } else {
        // Fallback to the original HTTP-only server if TLS is not configured.
        httpServer := &http.Server{
            Addr:    ":8080",
            Handler: http.HandlerFunc(mainHandler),
        }
        
        go func() {
            log.Println("Clara is ready. Starting HTTP server on :8080")
            if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
                log.Fatalf("HTTP Server ListenAndServe: %v", err)
            }
        }()

        stop := make(chan os.Signal, 1)
        signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
        <-stop

        log.Println("Shutting down Clara...")
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        if err := httpServer.Shutdown(ctx); err != nil {
            log.Fatalf("Graceful HTTP shutdown failed: %v", err)
        }
    }

    log.Println("Clara has gracefully shut down.")
}
