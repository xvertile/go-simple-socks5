package handler

import (
	"go-socks5/internal/proxy"
	"log"
	"net"
)

func HandleConnection(conn net.Conn) {
	if err := proxy.HandShake(conn); err != nil {
		log.Println("SOCKS5 handshake failed:", err)
		return
	}
	addr, err := proxy.ParseTarget(conn)
	if err != nil {
		log.Println("Failed to parse target address:", err)
		return
	}
	proxy.PipeWhenClose(conn, addr)
}
