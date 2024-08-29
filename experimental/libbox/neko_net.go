package libbox

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/sagernet/sing-box/adapter"
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
	defaultOutbound := box.Outbound().Default()
	conn, err := defaultOutbound.DialContext(ctx, network, metadata.ParseSocksaddr(addr))
	if err != nil {
		return nil, err
	}
	if box.clashServer != nil {
		conn = box.clashServer.RoutedConnection(ctx, conn, adapter.InboundContext{}, nil, defaultOutbound)
	}
	return conn, nil
}
