package proxy

import (
	"io"
	"log"
	"net"
	"time"
)

func PipeWhenClose(conn net.Conn, target string) {
	remoteConn, err := net.DialTimeout("tcp", target, 15*time.Second)
	if err != nil {
		log.Println("Failed to connect to remote target:", err)
		return
	}
	defer remoteConn.Close()
	tcpAddr := remoteConn.LocalAddr().(*net.TCPAddr)
	reply := make([]byte, 10)
	reply[0], reply[1], reply[2] = 0x05, 0x00, 0x00
	ip := tcpAddr.IP.To4()
	if ip == nil {
		ip = tcpAddr.IP.To16()
		reply[3] = 0x04
	} else {
		reply[3] = 0x01
	}
	copy(reply[4:], ip)
	reply[8] = byte(tcpAddr.Port >> 8)
	reply[9] = byte(tcpAddr.Port & 0xff)

	if _, err := conn.Write(reply); err != nil {
		log.Println("Failed to send response to client:", err)
		return
	}
	go io.Copy(remoteConn, conn)
	io.Copy(conn, remoteConn)
}
