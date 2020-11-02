package shared

import "net"

func GetServerAddr() *net.UDPAddr {
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:4500")
	if err != nil {
		panic(err)
	}
	return addr
}
