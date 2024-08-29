package main

import "C"

import (
	"context"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"time"

	"github.com/sagernet/sing-box/experimental/libbox"

	"github.com/matsuridayo/libneko/neko_common"
	"github.com/matsuridayo/libneko/speedtest"
)

var mainInstance *libbox.BoxInstance

//export BoxStart
func BoxStart(CoreConfig *C.char) *C.char {
	coreConfig := C.GoString(CoreConfig)

	if neko_common.Debug {
		log.Println("Start:", coreConfig)
	}

	if mainInstance != nil {
		return C.CString("instance already started")
	}

	instance, err := libbox.NewSingBoxInstance(coreConfig, false)
	if err == nil {
		err = instance.Start()
	}
	if err != nil {
		return C.CString(err.Error())
	}

	mainInstance = instance
	return nil
}

//export BoxStop
func BoxStop() *C.char {
	if mainInstance != nil {
		err := mainInstance.Close()
		mainInstance = nil
		if err != nil {
			return C.CString(err.Error())
		}
	}
	return nil
}

//export BoxTest
func BoxTest(Mode C.int, Address *C.char, Target *C.char, Timeout C.int, CoreConfig *C.char) *C.char {
	mode := int(Mode)
	address := C.GoString(Address)
	target := C.GoString(Target)
	timeout := int32(Timeout) * 1000 // seconds to milliseconds
	coreConfig := C.GoString(CoreConfig)

	const (
		TcpPing = iota
		HttpLatency
		DnsLatency
		DownloadSpeed
		IpLookup
	)
	const (
		KiB = 1024
		MiB = 1024 * KiB
	)

	var results string
	testResult := func(v any, err error) {
		if err != nil {
			results = err.Error()
		} else {
			results = fmt.Sprint(v)
		}
	}

	var (
		i          *libbox.BoxInstance
		httpClient *http.Client
	)
	if mode != TcpPing {
		if coreConfig != "" {
			// Test instance
			var err error
			i, err = libbox.NewSingBoxInstance(coreConfig, true)
			if err == nil {
				err = i.Start()
			}
			if err != nil {
				return C.CString(err.Error())
			}
			defer i.Close()
		} else {
			// Test running instance
			i = mainInstance
			if i == nil {
				return C.CString("no interface")
			}
		}
		httpClient = libbox.CreateProxyHttpClient(i)
	}

	// --- Tests ---

	switch mode {
	case TcpPing:
		ms, err := speedtest.TcpPing(address, timeout)
		testResult(ms, err)

	case HttpLatency:
		ms, err := speedtest.UrlTest(httpClient, target, timeout, speedtest.UrlTestStandard_Handshake)
		testResult(ms, err)

	case DnsLatency:
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Millisecond)
		defer cancel()

		start := time.Now()
		pc, err := libbox.DialContext(ctx, i, "udp", target)
		if err == nil {
			defer pc.Close()
			_ = pc.SetDeadline(time.Now().Add(time.Duration(timeout) * time.Millisecond))
			dnsQuery := []byte{
				0x00, 0x00, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x03, 'w', 'w', 'w', 0x06, 'g', 'o', 'o', 'g', 'l', 'e', 0x03, 'c', 'o', 'm', 0x00,
				0x00, 0x01, 0x00, 0x01,
			}
			if _, err = pc.Write(dnsQuery); err == nil {
				var buf [512]byte
				_, err = pc.Read(buf[:])
			}
		}
		testResult(time.Since(start).Milliseconds(), err)

	case DownloadSpeed:
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Millisecond)
		defer cancel()

		var n int64
		req, err := http.NewRequestWithContext(ctx, "GET", target, nil)
		start := time.Now()
		if err == nil {
			var resp *http.Response
			resp, err = httpClient.Do(req)
			if err == nil {
				defer resp.Body.Close()
				n, err = io.Copy(io.Discard, resp.Body)
			}
		}
		duration := math.Max(time.Since(start).Seconds(), 0.000001)
		testResult(float64(n)/duration/MiB, err)

	case IpLookup:
		var body []byte
		resp, err := httpClient.Get(target)
		if err == nil {
			defer resp.Body.Close()
			body, err = io.ReadAll(resp.Body)
		}
		testResult(string(body), err)

	default:
		results = "invalid test mode"
	}

	return C.CString(results)
}

//export BoxStats
func BoxStats() *C.char {
	if mainInstance != nil {
		stats, err := mainInstance.QueryStats()
		if err == nil {
			return C.CString(string(stats))
		}
	}
	return nil
}
