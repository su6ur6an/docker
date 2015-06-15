package utils

import (
	"fmt"
	"net"
	"net/http"
	"time"
    "path/filepath"
    "github.com/natefinch/npipe"
)

func ConfigureTCPTransport(tr *http.Transport, proto, addr string) {
	// Why 32? See https://github.com/docker/docker/pull/8035.
	timeout := 32 * time.Second
	if proto == "unix" {
		// No need for compression in local communications.
		tr.DisableCompression = true
		tr.Dial = func(_, _ string) (net.Conn, error) {
			return net.DialTimeout(proto, addr, timeout)
		}
	} else if proto == "npipe" {
                win32Path := fmt.Sprintf(`\\%s`, filepath.FromSlash(addr))
                tr.Dial = func(_, _ string) (net.Conn, error) {
                        return npipe.DialTimeout(win32Path, 50)
                }
	} else {
		tr.Proxy = http.ProxyFromEnvironment
		tr.Dial = (&net.Dialer{Timeout: timeout}).Dial
	}
}
