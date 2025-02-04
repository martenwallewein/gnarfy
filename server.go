package gnarfy

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"

	"github.com/google/uuid"
)

type HTTPServer struct {
	requestQueue  map[string]http.Request
	responseQueue map[string]*http.Response
	queueLock     sync.Mutex
}

func NewHTTPServer() *HTTPServer {
	return &HTTPServer{
		requestQueue:  make(map[string]http.Request),
		responseQueue: make(map[string]*http.Response),
	}
}

func (s *HTTPServer) handleExternalRequest(w http.ResponseWriter, r *http.Request) {
	// Generate a unique ID for the request
	requestID := uuid.New().String()

	// Store the incoming request in the queue
	s.queueLock.Lock()
	s.requestQueue[requestID] = *r
	s.queueLock.Unlock()

	fmt.Printf("[+] External request queued: %s %s (ID: %s)\n", r.Method, r.URL.Path, requestID)

	// Wait for the client to process the request
	for {
		s.queueLock.Lock()
		resp, exists := s.responseQueue[requestID]
		if exists {
			delete(s.responseQueue, requestID) // Clean up processed response
		}
		s.queueLock.Unlock()

		if exists {
			fmt.Println("Got response for id", requestID)
			// Read the full response body
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				http.Error(w, "Failed to read response body", http.StatusInternalServerError)
				return
			}
			resp.Body.Close() // Close the body after reading
			fmt.Println("Read response", len(body))
			// Write the response back to the external client
			for name, values := range resp.Header {
				for _, value := range values {
					w.Header().Add(name, value)
				}
			}
			// Retrieve the original status code from the header
			originalStatusCode := http.StatusOK // Default to 200
			statusCodeHeader := r.Header.Get("X-Original-Status-Code")
			if statusCodeHeader != "" {
				var err error
				originalStatusCode, err = strconv.Atoi(statusCodeHeader)
				if err != nil {
					fmt.Printf("[!] Invalid status code in header: %v\n", err)
					http.Error(w, "Invalid status code in header", http.StatusBadRequest)
					return
				}
			}
			w.WriteHeader(originalStatusCode)
			w.Write(body) // Write the buffered body
			return
		}
	}
}

func (s *HTTPServer) handleClientPoll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Invalid method", http.StatusMethodNotAllowed)
		return
	}

	// Send the next request in the queue to the client
	s.queueLock.Lock()
	for requestID, req := range s.requestQueue {
		delete(s.requestQueue, requestID) // Remove the request from the queue

		buffer := new(bytes.Buffer)
		io.Copy(buffer, req.Body)
		req.Body.Close()
		// Preserve the path and query string
		originalPath := req.URL.Path
		if req.URL.RawQuery != "" {
			originalPath += "?" + req.URL.RawQuery
		}

		w.Header().Set("Original-Method", req.Method)
		w.Header().Set("Original-Path", originalPath)
		w.Header().Set("Request-ID", requestID)
		w.Header().Set("Content-Type", req.Header.Get("Content-Type"))
		for name, values := range req.Header {
			for _, value := range values {
				w.Header().Set(name, value)
			}
		}

		w.Write(buffer.Bytes())
		s.queueLock.Unlock()
		return
	}
	s.queueLock.Unlock()

	http.Error(w, "No requests in the queue", http.StatusNoContent)
}

func (s *HTTPServer) handleClientResponse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid method", http.StatusMethodNotAllowed)
		return
	}

	requestID := r.Header.Get("Request-ID")
	if requestID == "" {
		http.Error(w, "Missing Request-ID in response", http.StatusBadRequest)
		return
	}

	// Read and copy the client's response body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read response body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	// Store the client's response in the response queue
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     r.Header.Clone(),
		Body:       io.NopCloser(bytes.NewReader(body)),
	}

	s.queueLock.Lock()
	s.responseQueue[requestID] = resp
	s.queueLock.Unlock()

	fmt.Printf("[+] Response received for request ID: %s\n", requestID)
	w.WriteHeader(http.StatusOK)
}

func (s *HTTPServer) Run(externalListenAddr, clientListenAddr string) {
	http.HandleFunc("/poll", s.handleClientPoll)
	http.HandleFunc("/response", s.handleClientResponse)
	http.HandleFunc("/", s.handleExternalRequest)

	fmt.Printf("[*] Listening for external clients on %s and for internal clients on %s\n", externalListenAddr, clientListenAddr)

	go func() {
		if err := http.ListenAndServe(clientListenAddr, nil); err != nil {
			fmt.Printf("Error starting client listener: %v\n", err)
			os.Exit(1)
		}
	}()

	if err := http.ListenAndServe(externalListenAddr, nil); err != nil {
		fmt.Printf("Error starting external listener: %v\n", err)
		os.Exit(1)
	}
}
