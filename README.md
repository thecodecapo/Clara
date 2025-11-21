# Clara: The Clear & Declarative Reverse Proxy


Clara is a modern, open-source reverse proxy built in Go, designed for simplicity, clarity, and performance. Unlike traditional proxies that rely on complex, imperative scripts, Clara uses a simple declarative YAML file to manage your routing.

Just declare the state you want, and Clara handles the rest.

* * * * *

## ✨ Features


-   ✅ **Declarative YAML Configuration:** No complex syntax. Just a clean, human-readable file.

-   ✅ **Automatic HTTPS:** Powered by Let's Encrypt, Clara automatically provisions and renews TLS certificates.

-   ✅ **Zero-Downtime Hot Reloading:** Change your configuration and Clara reloads it on the fly without interrupting service.

-   ✅ **Round-Robin Load Balancing:** Distribute traffic across multiple backend servers for scalability and redundancy.

-   ✅ **Prometheus Metrics:** An embedded `/metrics` endpoint provides detailed observability into traffic, latency, and errors.

-   ✅ **Polished User Experience:** Comes with a built-in welcome page and a default 404 page, both fully customizable.

-   ✅ **High Performance & Low Footprint:** Built in Go to be fast, lightweight, and highly concurrent.

-   ✅ **Graceful Shutdown:** Protects against dropped connections during restarts or deployments.

* * * * *

## 🚀 Installation

###  Note : Run Clara as a system service for production use, see below section ( Running Clara as a Service )


Clara is distributed as a single, standalone binary. No dependencies are needed.

### Easiest Install (Linux & macOS)

Run this command in your terminal to automatically download and install the latest version of Clara.

```bash
curl -sSL https://github.com/thecodecapo/Clara/raw/main/install.sh | sudo sh

```



## ⚙️ Configuration


Clara is configured using a single `config.yaml` file. The binary will search for this file in the current directory, `~/.config/clara/`, or `/etc/clara/`.

Here is a complete example showcasing all features:

YAML

```
# --- TLS Configuration (Optional) ---
# Enable automatic HTTPS with Let's Encrypt.
tls:
  email: "your-email@example.com"
  domains:
    - "your.domain.com"

# --- Services ---
# A service can be a single host or multiple servers for load balancing.
services:
  - name: "my-api-service"
    load_balancing_strategy: "round-robin"
    servers:
      - "http://localhost:4001"
      - "http://localhost:4002"

  - name: "my-website"
    host: "localhost"
    port: 3000

# --- Routes ---
# Map incoming paths to your services.
routes:
  - path: "/api/"
    service: "my-api-service"

  - path: "/"
    service: "my-website"

# --- Error Pages (Optional) ---
# Override the built-in default pages with your own.
error_pages:
  404: "./pages/404.html"

```

* * * * *

## 🏃 Running Clara as a Service (Linux)

For production deployments on Linux, it's best to run Clara as a systemd service. This allows it to start on boot and restart automatically if it crashes.

- Install the Binary: First, install Clara using the one-line install script.


```
curl -sSL https://raw.githubusercontent.com/thecodecapo/Clara/main/install.sh | sudo sh

```
- Install the Service: Use the built-in -install command to create the service file.

```
sudo clara -install
```

- Enable and Start the Service: Follow the on-screen instructions to get the service running.

```
sudo systemctl daemon-reload
sudo systemctl enable clara
sudo systemctl start clara
```

- Check the Status: You can check that the service is running at any time.

```
sudo systemctl status clara
```

## 📈 Observability


Clara exposes a Prometheus metrics endpoint by default on port **9091**.

-   **Endpoint:** `http://localhost:9091/metrics`

-   **Exposed Metrics:**

    -   `clara_http_requests_total`: Total requests, labeled by service, path, and status code.

    -   `clara_http_request_duration_seconds`: A histogram of request latency, labeled by service and path.

    -   Standard Go runtime and process metrics.

* * * * *

## 🛣️ Project Roadmap


Clara is actively developed. Here's what's planned for the future:

-   [ ] **More Load Balancing Strategies:** Least connections, IP Hash, etc.

-   [ ] **Websocket Support:** Full support for proxying websocket connections.

-   [ ] **Plugin System:** Extend Clara's functionality with custom middleware.

* * * * *

## 🙌 Contributing


We welcome contributions of all kinds! If you're interested in helping, please:

1.  Fork the repository.

2.  Create a new branch for your feature or bugfix.

3.  Open a pull request with a clear description of your changes.

If you find a bug or have a feature request, please [open an issue](https://github.com/thecodecapo/Clara/issues).
