package gnarfy

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTPClient represents the client behind the NAT
type HTTPClient struct {
	serverURL string
	targetURL string
}

func NewHTTPClient(serverURL, targetURL string) *HTTPClient {
	return &HTTPClient{
		serverURL: serverURL,
		targetURL: targetURL,
	}
}

func (c *HTTPClient) pollServer() (*http.Request, string, error) {
	resp, err := http.Get(c.serverURL + "/poll")
	if err != nil {
		return nil, "", fmt.Errorf("failed to poll server: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil, "", nil // No requests available
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read polled request: %v", err)
	}
	fmt.Println("Got body ", string(body))

	requestID := resp.Header.Get("Request-ID")
	if requestID == "" {
		return nil, "", fmt.Errorf("missing Request-ID in polled request")
	}

	// Read the original method from the headers
	originalMethod := resp.Header.Get("Original-Method")
	if originalMethod == "" {
		return nil, "", fmt.Errorf("missing Original-Method in polled request")
	}

	originalPath := resp.Header.Get("Original-Path")
	if originalMethod == "" || originalPath == "" {
		return nil, "", fmt.Errorf("missing Original-Method or Original-Path in polled request")
	}

	fmt.Println(originalPath)

	req, err := http.NewRequest(originalMethod, c.targetURL+originalPath, bytes.NewReader(body))
	req.Header.Add("Content-Type", resp.Header.Get("Content-Type"))

	for name, values := range resp.Header {
		for _, value := range values {
			req.Header.Set(name, value)
		}
	}

	return req, requestID, err
}

func (c *HTTPClient) sendResponse(req *http.Request, requestID string) error {
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to forward request to target: %v", err)
	}
	defer resp.Body.Close()

	// Send the response back to the server
	serverReq, err := http.NewRequest(http.MethodPost, c.serverURL+"/response", resp.Body)
	if err != nil {
		return fmt.Errorf("failed to create server response: %v", err)
	}

	// Set headers from the target server response
	for name, values := range resp.Header {
		for _, value := range values {
			serverReq.Header.Add(name, value)
		}
	}
	serverReq.Header.Set("Request-ID", requestID)
	serverReq.Header.Set("Content-Type", resp.Header.Get("Content-Type"))
	// Set the status code of the original response in a custom header (if needed, since HTTP doesn't support sending a status code as a header)
	serverReq.Header.Set("X-Original-Status-Code", fmt.Sprintf("%d", resp.StatusCode))
	fmt.Println(serverReq.Header.Get("Response-code"))
	fmt.Println(resp.StatusCode)
	serverResp, err := http.DefaultClient.Do(serverReq)
	if err != nil {
		return fmt.Errorf("failed to send response to server: %v", err)
	}
	serverResp.Body.Close()
	return nil
}

func (c *HTTPClient) Run() {
	fmt.Println("[*] Client started, polling for requests...")
	for {
		time.Sleep(50 * time.Millisecond)
		req, requestID, err := c.pollServer()
		if err != nil {
			fmt.Printf("Error polling server: %v\n", err)
			continue
		}
		if req == nil {
			continue // No requests available, keep polling
		}

		if err := c.sendResponse(req, requestID); err != nil {
			fmt.Printf("Error sending response: %v\n", err)
		}
	}
}
