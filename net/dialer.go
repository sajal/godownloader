package godownloader

//Backfill until Go gives us a real DialTCPTimeout method

import (
	"fmt"
	"net"
	"time"
)

type Dialresponse struct {
	Conn *net.TCPConn
	Err  error
}

func dialer(netw string, srcaddr, tcpaddr *net.TCPAddr, c chan Dialresponse) {
	conn, err := net.DialTCP(netw, srcaddr, tcpaddr)
	response := Dialresponse{Conn: conn, Err: err}
	c <- response
	//return c
}

func DialTCPTimeout(netw string, laddr, raddr *net.TCPAddr) (*net.TCPConn, error) {
	//Do this hanky panky cause there is no DialTCPTimeout in net
	var conn *net.TCPConn
	var err error
	c := make(chan Dialresponse)
	go dialer(netw, laddr, raddr, c)
	select {
	case response := <-c:
		conn = response.Conn
		err = response.Err
		//return conn, err
	case <-time.After(20 * time.Second):
		err = fmt.Errorf("FAIL conn from %s to %s took too long", laddr, raddr)

	}
	return conn, err
}