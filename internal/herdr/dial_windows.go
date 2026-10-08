//go:build windows

package herdr

import (
	"errors"
	"io/fs"
	"net"
	"time"

	"github.com/Microsoft/go-winio"
)

// dial connects to the Herdr server. On Windows Herdr listens on a named pipe
// called \\.\pipe\<HERDR_SOCKET_PATH>; the file at that path is only a marker.
// When no such pipe exists (the unix-socket fake the tests start), it falls
// back to the unix socket.
func dial(socketPath string, timeout time.Duration) (net.Conn, error) {
	conn, err := winio.DialPipe(`\\.\pipe\`+socketPath, &timeout)
	if err == nil {
		return conn, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return net.DialTimeout("unix", socketPath, timeout)
	}
	return nil, err
}
