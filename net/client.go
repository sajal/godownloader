package godownloader

import (
	"log"
	"net"
	"net/http"
	"time"
)

// Returns a http.Client bounded to the network interface that laddr belongs to
// All requests going out use laddr as the source address
func GetClient(laddr net.IP, timeout time.Duration) *http.Client {
	tr := &http.Transport{
		Dial: func(network string, addr string) (net.Conn, error) {
			tcpaddr, err := net.ResolveTCPAddr(network, addr)
			if err != nil {
				log.Panic(err)
			}
			//fmt.Println(addr)
			//Picking port number as 0 gets a free port. http://osdir.com/ml/go-language-discuss/2013-05/msg01285.html
			srcaddr := &net.TCPAddr{IP: laddr, Port: 0}
			//Dial tcp with custom source address ..
			return DialTCPTimeout("tcp", srcaddr, tcpaddr, timeout)
		},
		ResponseHeaderTimeout: timeout, //If headers not received in timeout then FAIL
	}
	client := &http.Client{Transport: tr}
	return client
}
