# 4Word - Simple HTTP Proxy Server

4Word is a lightweight HTTP proxy server built in Go that acts as a reverse proxy for web requests. It's designed to be simple to use and supports both regular HTTP requests and streaming responses.

## Features

- Forward HTTP requests to any URL by specifying the target domain in the path
- Automatic HTTPS upgrade (defaults to HTTPS if no protocol specified)
- Support for streaming responses (SSE, NDJSON, multipart)
- Preserves all request headers and body
- Proper handling of redirects
- Zero configuration required
- No request timeouts for long-running connections
- Built-in compression handling

## Usage

### Running the server

```bash
# Run with default port (4444)
go run main.go

# Run with a custom port
PORT=3000 go run main.go
```

### Making requests

The proxy works by including the target domain in the path of your request:

```bash
# Target https://google.com
curl -X GET http://localhost:4444/google.com

# Target https://api.example.com/data
curl -X POST http://localhost:4444/api.example.com/data -H "Content-Type: application/json" -d '{"key": "value"}'

# With query parameters
curl -X GET http://localhost:4444/google.com/search?q=hello+world

# Explicitly specify protocol (http or https)
curl -X GET http://localhost:4444/http://example.com/path
```

### Streaming Support

4Word has built-in support for streaming responses, including:

- Server-Sent Events (SSE)
- NDJSON streams
- Multipart responses

The proxy will automatically detect streaming responses and handle them appropriately without buffering.

### Web Interface

When accessing the proxy without a target URL (e.g., http://localhost:4444/), it will serve an index.html file from the same directory as the binary. You can customize this page to create a web interface for your proxy.

## Building and Deployment

### Build the binary

```bash
go build -o 4word
```

### Docker

You can containerize the application using Docker:

```bash
docker build -t 4word .
docker run -p 4444:4444 4word
```

## Environment Variables

- `PORT`: The port on which the server listens (default: 4444)

## Security Considerations

- The proxy currently forwards all requests without authentication
- Consider implementing access controls for production use
- Be aware that the proxy can be used to access any publicly available HTTP(S) endpoint

## License

MIT
