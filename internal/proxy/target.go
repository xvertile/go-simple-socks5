package proxy

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strconv"
)

var (
	errAddrType     = errors.New("SOCKS address type not supported")
	errCmd          = errors.New("SOCKS only supports CONNECT command")
	errReqExtraData = errors.New("SOCKS request has extra data")
)

const socksCmdConnect = 0x01

func ParseTarget(conn net.Conn) (string, error) {
	const (
		idVer   = 0
		idCmd   = 1
		idType  = 3
		idIP0   = 4
		idDmLen = 4
		idDm0   = 5

		typeIPv4 = 1
		typeDm   = 3
		typeIPv6 = 4

		lenIPv4   = 10
		lenIPv6   = 22
		lenDmBase = 7
	)

	buf := make([]byte, 263)
	n, err := io.ReadAtLeast(conn, buf, idDmLen+1)
	if err != nil {
		return "", err
	}

	if buf[idVer] != socksVer5 {
		return "", errVer
	}
	if buf[idCmd] != socksCmdConnect {
		return "", errCmd
	}

	var reqLen int
	switch buf[idType] {
	case typeIPv4:
		reqLen = lenIPv4
	case typeIPv6:
		reqLen = lenIPv6
	case typeDm:
		reqLen = int(buf[idDmLen]) + lenDmBase
	default:
		return "", errAddrType
	}

	if n < reqLen {
		if _, err := io.ReadFull(conn, buf[n:reqLen]); err != nil {
			return "", err
		}
	} else if n > reqLen {
		return "", errReqExtraData
	}

	var host string
	switch buf[idType] {
	case typeIPv4:
		host = net.IP(buf[idIP0 : idIP0+net.IPv4len]).String()
	case typeIPv6:
		host = net.IP(buf[idIP0 : idIP0+net.IPv6len]).String()
	case typeDm:
		host = string(buf[idDm0 : idDm0+buf[idDmLen]])
	}

	port := binary.BigEndian.Uint16(buf[reqLen-2:])
	return net.JoinHostPort(host, strconv.Itoa(int(port))), nil
}
