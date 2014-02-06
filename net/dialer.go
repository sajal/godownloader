package godownloader

//Backfill until Go gives us a real DialTCPTimeout method

import (
	"fmt"
	"net"
	"time"
)

type dialresponse struct {
	Conn *net.TCPConn
	Err  error
}

func dialer(netw string, srcaddr, tcpaddr *net.TCPAddr, c chan dialresponse) {
	conn, err := net.DialTCP(netw, srcaddr, tcpaddr)
	response := dialresponse{Conn: conn, Err: err}
	c <- response
	//return c
}

// net.DialTCP but accept a timeout net.DialTimeout doesnt take laddr..
// net.DialTCP doesnt take a timeout
func DialTCPTimeout(netw string, laddr, raddr *net.TCPAddr, timeout time.Duration) (*net.TCPConn, error) {
	//Do this hanky panky cause there is no DialTCPTimeout in net
	var conn *net.TCPConn
	var err error
	c := make(chan dialresponse)
	go dialer(netw, laddr, raddr, c)
	select {
	case response := <-c:
		conn = response.Conn
		err = response.Err
		//return conn, err
	case <-time.After(timeout):
		err = fmt.Errorf("FAIL conn from %s to %s took too long", laddr, raddr)

	}
	return conn, err
}