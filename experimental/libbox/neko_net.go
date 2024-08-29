package libbox

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/sagernet/sing/common/metadata"
)

func CreateProxyHttpClient(box *BoxInstance) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return DialContext(ctx, box, network, addr)
			},
			TLSHandshakeTimeout:   time.Second * 3,
			ResponseHeaderTimeout: time.Second * 3,
		},
	}
}

func DialContext(ctx context.Context, box *BoxInstance, network, addr string) (net.Conn, error) {
	return box.Outbound().Default().DialContext(ctx, network, metadata.ParseSocksaddr(addr))
}
