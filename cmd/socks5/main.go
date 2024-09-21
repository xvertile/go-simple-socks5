package main

import (
	"go-socks5/internal/handler"
	"log"
	"net"
)

func main() {
	addr := ":8080"
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		panic(err)
	}
	log.Printf("Listening on %s\n", addr)
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println("Error accepting connection:", err)
			continue
		}
		go handler.HandleConnection(conn)
	}
}
