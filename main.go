package main

import (
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// extractTargetHost extracts the target host from the path
func extractTargetHost(path string) (string, string) {
	// Remove leading slash
	path = strings.TrimPrefix(path, "/")

	// If path is empty, return empty string
	if path == "" {
		return "", ""
	}

	// Find the first slash after removing the leading slash
	slashIndex := strings.Index(path, "/")

	// If there's no slash, the entire path is the host
	if slashIndex == -1 {
		return path, "/"
	}

	// Extract host and remaining path
	host := path[:slashIndex]
	remainingPath := path[slashIndex:]

	return host, remainingPath
}

// serveIndexHTML serves the index.html file
func serveIndexHTML(w http.ResponseWriter, r *http.Request) {
	// Get the path to the index.html file
	indexPath := filepath.Join(".", "index.html")

	// Check if the file exists
	_, err := os.Stat(indexPath)
	if os.IsNotExist(err) {
		http.Error(w, "Index file not found", http.StatusInternalServerError)
		return
	}

	// Serve the file
	http.ServeFile(w, r, indexPath)
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers for all responses
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Access-Control-Max-Age", "86400")

	// Handle OPTIONS request
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	path := r.URL.Path

	// If the path is just "/", serve the index.html file
	if path == "/" {
		serveIndexHTML(w, r)
		return
	}

	// Extract target host and remaining path
	targetHost, remainingPath := extractTargetHost(path)

	// If no target host is found, serve the index.html file
	if targetHost == "" {
		serveIndexHTML(w, r)
		return
	}

	// Check if the target host has a valid domain extension
	// This checks if there's at least one dot in the host name (e.g., "google.com")
	if !strings.Contains(targetHost, ".") {
		serveIndexHTML(w, r)
		return
	}

	// Prepend https:// if no protocol is specified
	var targetURL string
	if strings.HasPrefix(targetHost, "http://") || strings.HasPrefix(targetHost, "https://") {
		// Use protocol from target host
		if strings.HasPrefix(targetHost, "http://") {
			targetHost = strings.TrimPrefix(targetHost, "http://")
			targetURL = "http://" + targetHost
		} else {
			targetHost = strings.TrimPrefix(targetHost, "https://")
			targetURL = "https://" + targetHost
		}
	} else {
		// Default to https
		targetURL = "https://" + targetHost
	}

	// Append the remaining path
	fullURL := targetURL + remainingPath

	// Append query string if any
	if r.URL.RawQuery != "" {
		fullURL += "?" + r.URL.RawQuery
	}

	// Create new request with the same method, URL and body
	proxyReq, err := http.NewRequest(r.Method, fullURL, r.Body)
	if err != nil {
		http.Error(w, "Error creating proxy request: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Copy all headers from the original request
	for name, values := range r.Header {
		// Skip host header as we set it separately
		if strings.ToLower(name) == "host" {
			continue
		}
		for _, value := range values {
			proxyReq.Header.Add(name, value)
		}
	}

	// Add X-Forwarded headers
	proxyReq.Header.Set("X-Forwarded-Host", r.Host)
	proxyReq.Header.Set("X-Forwarded-Proto", "http")

	// Add host header for the target
	proxyReq.Host = targetHost

	// Create HTTP client with redirect handling
	// Remove timeout for streaming responses
	client := &http.Client{
		// No timeout set to allow for long-running connections like SSE
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Get the location header
			location := req.Response.Header.Get("Location")

			// If the location is a relative URL, let the default redirect handling work
			if !strings.HasPrefix(location, "http://") && !strings.HasPrefix(location, "https://") {
				return nil
			}

			// Parse the location URL
			locationURL, err := url.Parse(location)
			if err != nil {
				return err
			}

			// Modify the location to include the proxy
			newLocation := "/" + locationURL.Host
			if locationURL.Path != "" {
				newLocation += locationURL.Path
			}
			if locationURL.RawQuery != "" {
				newLocation += "?" + locationURL.RawQuery
			}

			// Set the modified location in the header
			req.Response.Header.Set("Location", newLocation)

			return nil
		},
		// Disable compression to avoid buffering
		Transport: &http.Transport{
			DisableCompression: true,
		},
	}

	// Send the request but don't close the connection immediately
	resp, err := client.Do(proxyReq)
	if err != nil {
		http.Error(w, "Error forwarding request: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Check if this is a streaming response
	isStreaming := false
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/event-stream") ||
	   strings.Contains(contentType, "application/x-ndjson") ||
	   strings.Contains(contentType, "multipart/") {
		isStreaming = true
	}

	// Copy response headers
	for name, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}

	// For streaming responses, ensure we're not buffering
	if isStreaming {
		// For event streams, make sure we disable any potential buffering
		if f, ok := w.(http.Flusher); ok {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")

			// Set status code
			w.WriteHeader(resp.StatusCode)

			// Create a buffer for more efficient reading
			buf := make([]byte, 4096)

			// Stream the response body in chunks and flush after each chunk
			for {
				n, err := resp.Body.Read(buf)
				if n > 0 {
					_, writeErr := w.Write(buf[:n])
					if writeErr != nil {
						log.Printf("Error writing response chunk: %v", writeErr)
						break
					}
					f.Flush() // Flush after each chunk
				}
				if err != nil {
					if err != io.EOF {
						log.Printf("Error reading response body: %v", err)
					}
					break
				}
			}
			return
		}
	}

	// Non-streaming case or if flushing isn't supported
	// Set status code
	w.WriteHeader(resp.StatusCode)

	// Copy response body
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		log.Printf("Error copying response body: %v", err)
	}
}

func main() {
	// Default port
	port := os.Getenv("PORT")
	if port == "" {
		port = "4444"
	}

	// Register proxy handler for all paths
	http.HandleFunc("/", proxyHandler)

	// Start server
	log.Printf("4Word proxy server starting on port %s...", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
