package main

import (
	"fmt"
	"os"

	"github.com/martenwallewein/gnarfy"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go [server|client]")
		os.Exit(1)
	}

	mode := os.Args[1]

	if mode == "server" {
		if len(os.Args) < 4 {
			fmt.Println("Usage: go run main.go server <client_listen_addr> <external_list_addr>")
			os.Exit(1)
		}
		clientListenAddr := os.Args[2]
		externalAddr := os.Args[3]
		server := gnarfy.NewHTTPServer()
		server.Run(externalAddr, clientListenAddr)
	} else if mode == "client" {
		if len(os.Args) < 4 {
			fmt.Println("Usage: go run main.go client <server_url> <target_url>")
			os.Exit(1)
		}
		serverURL := os.Args[2]
		targetURL := os.Args[3]
		client := gnarfy.NewHTTPClient(serverURL, targetURL)
		client.Run()
	} else {
		fmt.Println("Invalid mode. Use 'server' or 'client'.")
		os.Exit(1)
	}
}
