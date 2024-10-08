/*
Socks5 proxy server by golang
http://github.com/ring04h/s5.go

reference: shadowsocks go local.go
https://github.com/shadowsocks/shadowsocks-go
*/
package main

import (
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"runtime"
	"strconv"
	"time"
)

var (
	Commands         = []string{"CONNECT", "BIND", "UDP ASSOCIATE"}
	AddrType         = []string{"", "IPv4", "", "Domain", "IPv6"}
	Conns            = make([]net.Conn, 0)
	Verbose          = false
	DoAuth           = false
	errAddrType      = errors.New("socks addr type not supported")
	errVer           = errors.New("socks version not supported")
	errMethod        = errors.New("socks only support noauth method")
	errAuthExtraData = errors.New("socks authentication get extra data")
	errReqExtraData  = errors.New("socks request get extra data")
	errCmd           = errors.New("socks only support connect command")
)

const (
	socksVer5       = 0x05
	socksCmdConnect = 0x01
)

func netCopy(input, output net.Conn) (err error) {
	buf := make([]byte, 8192)
	for {
		count, err := input.Read(buf)
		if err != nil {
			if err == io.EOF && count > 0 {
				output.Write(buf[:count])
			}
			break
		}
		if count > 0 {
			output.Write(buf[:count])
		}
	}
	return
}

func handShake(conn net.Conn) (err error) {
	const (
		idVer     = 0
		idNmethod = 1
	)

	buf := make([]byte, 258)

	var n int

	// make sure we get the nmethod field
	if n, err = io.ReadAtLeast(conn, buf, idNmethod+1); err != nil {
		return
	}

	if buf[idVer] != socksVer5 {
		return errVer
	}

	nmethod := int(buf[idNmethod]) //  client support auth mode
	msgLen := nmethod + 2          //  auth msg length
	if n == msgLen {               // handshake done, common case
		// do nothing, jump directly to send confirmation
	} else if n < msgLen { // has more methods to read, rare case
		if _, err = io.ReadFull(conn, buf[n:msgLen]); err != nil {
			return
		}
	} else { // error, should not get extra data
		return errAuthExtraData
	}
	/*
	   X'00' NO AUTHENTICATION REQUIRED
	   X'01' GSSAPI
	   X'02' USERNAME/PASSWORD
	   X'03' to X'7F' IANA ASSIGNED
	   X'80' to X'FE' RESERVED FOR PRIVATE METHODS
	   X'FF' NO ACCEPTABLE METHODS
	*/
	// send confirmation: version 5, no authentication required
	if DoAuth {
		_, err = conn.Write([]byte{socksVer5, 2})
	} else {
		_, err = conn.Write([]byte{socksVer5, 0})
	}
	return
}

func parseTarget(conn net.Conn) (host string, err error) {
	const (
		idVer   = 0
		idCmd   = 1
		idType  = 3 // address type index
		idIP0   = 4 // ip addres start index
		idDmLen = 4 // domain address length index
		idDm0   = 5 // domain address start index

		typeIPv4 = 1 // type is ipv4 address
		typeDm   = 3 // type is domain address
		typeIPv6 = 4 // type is ipv6 address

		lenIPv4   = 3 + 1 + net.IPv4len + 2 // 3(ver+cmd+rsv) + 1addrType + ipv4 + 2port
		lenIPv6   = 3 + 1 + net.IPv6len + 2 // 3(ver+cmd+rsv) + 1addrType + ipv6 + 2port
		lenDmBase = 3 + 1 + 1 + 2           // 3 + 1addrType + 1addrLen + 2port, plus addrLen
	)
	// refer to getRequest in server.go for why set buffer size to 263
	buf := make([]byte, 263)
	var n int

	// read till we get possible domain length field
	if n, err = io.ReadAtLeast(conn, buf, idDmLen+1); err != nil {
		return
	}

	// check version and cmd
	if buf[idVer] != socksVer5 {
		err = errVer
		return
	}

	/*
	   CONNECT X'01'
	   BIND X'02'
	   UDP ASSOCIATE X'03'
	*/

	if buf[idCmd] > 0x03 || buf[idCmd] == 0x00 {
		log.Println("Unknown Command", buf[idCmd])
	}

	if Verbose {
		log.Println("Command:", Commands[buf[idCmd]-1])
	}

	if buf[idCmd] != socksCmdConnect { //  only support CONNECT mode
		err = errCmd
		return
	}

	// read target address
	reqLen := -1
	switch buf[idType] {
	case typeIPv4:
		reqLen = lenIPv4
	case typeIPv6:
		reqLen = lenIPv6
	case typeDm: // domain name
		reqLen = int(buf[idDmLen]) + lenDmBase
	default:
		err = errAddrType
		return
	}

	if n == reqLen {
		// common case, do nothing
	} else if n < reqLen { // rare case
		if _, err = io.ReadFull(conn, buf[n:reqLen]); err != nil {
			return
		}
	} else {
		err = errReqExtraData
		return
	}

	switch buf[idType] {
	case typeIPv4:
		host = net.IP(buf[idIP0 : idIP0+net.IPv4len]).String()
	case typeIPv6:
		host = net.IP(buf[idIP0 : idIP0+net.IPv6len]).String()
	case typeDm:
		host = string(buf[idDm0 : idDm0+buf[idDmLen]])
	}
	port := binary.BigEndian.Uint16(buf[reqLen-2 : reqLen])
	host = net.JoinHostPort(host, strconv.Itoa(int(port)))

	return
}

func pipeWhenClose(conn net.Conn, target string) {

	if Verbose {
		log.Println("Connect remote ", target, "...")
	}

	remoteConn, err := net.DialTimeout("tcp", target, time.Duration(time.Second*15))
	if err != nil {
		log.Println("Connect remote :", err)
		return
	}

	tcpAddr := remoteConn.LocalAddr().(*net.TCPAddr)
	if tcpAddr.Zone == "" {
		if tcpAddr.IP.Equal(tcpAddr.IP.To4()) {
			tcpAddr.Zone = "ip4"
		} else {
			tcpAddr.Zone = "ip6"
		}
	}

	if Verbose {
		log.Println("Connect remote success @", tcpAddr.String())
	}

	rep := make([]byte, 256)
	rep[0] = 0x05
	rep[1] = 0x00 // success
	rep[2] = 0x00 //RSV

	//IP
	if tcpAddr.Zone == "ip6" {
		rep[3] = 0x04 //IPv6
	} else {
		rep[3] = 0x01 //IPv4
	}

	var ip net.IP
	if "ip6" == tcpAddr.Zone {
		ip = tcpAddr.IP.To16()
	} else {
		ip = tcpAddr.IP.To4()
	}
	pindex := 4
	for _, b := range ip {
		rep[pindex] = b
		pindex += 1
	}
	rep[pindex] = byte((tcpAddr.Port >> 8) & 0xff)
	rep[pindex+1] = byte(tcpAddr.Port & 0xff)
	conn.Write(rep[0 : pindex+2])
	// Transfer data

	defer remoteConn.Close()

	// Copy local to remote
	go netCopy(conn, remoteConn)

	// Copy remote to local
	netCopy(remoteConn, conn)
}

func handleAuth(conn net.Conn) (err error) {
	const (
		idVer = 0
	)
	buf := make([]byte, 258)

	if _, err := io.ReadAtLeast(conn, buf, 4); err != nil {
		return err
	}
	if buf[idVer] != 0x01 {
		return errors.New("auth version not supported")
	}
	offset := 2
	if len(buf) < offset+int(buf[offset-1]) {
		return errors.New("auth data length error")

	}
	username := string(buf[offset : int(buf[offset-1])+offset])
	offset += int(buf[offset-1]) + 1
	if len(buf) < offset+int(buf[offset-1]) {
		return errors.New("auth data length error")

	}
	password := string(buf[offset : int(buf[offset-1])+offset])
	fmt.Println("username:", string(username))
	fmt.Println("password:", string(password))
	if username == "admin" && password == "test" {
		conn.Write([]byte{socksVer5, 0x00})
		return nil
	}
	conn.Write([]byte{socksVer5, 0x01})
	return errors.New("auth failed")

}

func handleConnection(conn net.Conn) {
	Conns = append(Conns, conn)
	defer func() {
		for i, c := range Conns {
			if c == conn {
				Conns = append(Conns[:i], Conns[i+1:]...)
			}
		}
		conn.Close()
	}()
	if err := handShake(conn); err != nil {
		log.Println("socks handshake:", err)
		return
	}
	if DoAuth {
		if err := handleAuth(conn); err != nil {
			log.Println("socks auth:", err)
			return
		}
	}
	addr, err := parseTarget(conn)
	if err != nil {
		log.Println("socks consult transfer mode or parse target :", err)
		return
	}
	pipeWhenClose(conn, addr)
}

func main() {

	// maxout concurrency
	runtime.GOMAXPROCS(runtime.NumCPU())

	verbose := flag.Bool("v", false, "should every proxy request be logged to stdout")
	doauth := flag.Bool("a", false, "should do auth")

	addr := flag.String("addr", ":8080", "proxy listen address")
	flag.Parse()

	Verbose = *verbose
	DoAuth = *doauth
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		panic(err)
		return
	}

	log.Printf("Listening %s \n", *addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Println(err)
			return
		}
		if Verbose {
			log.Println("new client:", conn.RemoteAddr())
		}
		go handleConnection(conn)
	}
}
