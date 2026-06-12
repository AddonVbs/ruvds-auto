package probe

import (
	"net"
	"strconv"
	"time"
)

// Common ports to test reachability. If any responds, host is considered alive.
var defaultPorts = []int{22, 3389, 80, 443}

type Result struct {
	IP    string
	Alive bool
	Port  int // port that answered, 0 if none
}

// Check tries a TCP handshake on a few common ports. No root required.
func Check(ip string, timeout time.Duration) Result {
	for _, p := range defaultPorts {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, strconv.Itoa(p)), timeout)
		if err == nil {
			_ = conn.Close()
			return Result{IP: ip, Alive: true, Port: p}
		}
	}
	return Result{IP: ip, Alive: false}
}

// CheckAll probes a slice of IPs in parallel.
func CheckAll(ips []string, timeout time.Duration) []Result {
	out := make([]Result, len(ips))
	ch := make(chan struct {
		i int
		r Result
	}, len(ips))
	for i, ip := range ips {
		go func(i int, ip string) {
			ch <- struct {
				i int
				r Result
			}{i, Check(ip, timeout)}
		}(i, ip)
	}
	for range ips {
		x := <-ch
		out[x.i] = x.r
	}
	return out
}
