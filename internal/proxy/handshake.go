package proxy

import (
	"errors"
	"io"
	"net"
)

const socksVer5 = 0x05

var (
	errVer           = errors.New("SOCKS version not supported")
	errAuthExtraData = errors.New("SOCKS authentication received extra data")
)

func HandShake(conn net.Conn) error {
	buf := make([]byte, 258)

	n, err := io.ReadAtLeast(conn, buf, 2)
	if err != nil {
		return err
	}

	if buf[0] != socksVer5 {
		return errVer
	}

	nmethods := int(buf[1])
	msgLen := nmethods + 2

	if n < msgLen {
		if _, err = io.ReadFull(conn, buf[n:msgLen]); err != nil {
			return err
		}
	} else if n > msgLen {

		return errAuthExtraData
	}

	_, err = conn.Write([]byte{socksVer5, 0x00})
	return err
}
