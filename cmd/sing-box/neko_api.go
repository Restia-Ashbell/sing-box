package main

import "C"

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
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
func BoxTest(Mode C.int, Address *C.char, Url *C.char, Timeout C.int, SpeedUrl *C.char, SpeedTimeout C.int, CoreConfig *C.char) *C.char {
	mode := int(Mode)
	address := C.GoString(Address)
	url := C.GoString(Url)
	timeout := int32(Timeout)
	speedUrl := C.GoString(SpeedUrl)
	speedTimeout := int32(SpeedTimeout)
	coreConfig := C.GoString(CoreConfig)
	const (
		TcpPing   = 1 << 0 // 1
		UrlTest   = 1 << 1 // 2
		UdpTest   = 1 << 2 // 4
		SpeedTest = 1 << 3 // 8
		IpTest    = 1 << 4 // 16
	)
	const (
		KiB = 1024
		MiB = 1024 * KiB
	)
	var (
		i          *libbox.BoxInstance
		httpClient *http.Client
		results    []string
	)
	if mode&(UrlTest|UdpTest|SpeedTest|IpTest) != 0 {
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
	if mode&TcpPing != 0 {
		ms, err := speedtest.TcpPing(address, timeout)
		if err == nil {
			results = append(results, strconv.Itoa(int(ms)))
		} else {
			results = append(results, err.Error())
		}
	}
	if mode&UrlTest != 0 {
		ms, err := speedtest.UrlTest(httpClient, url, timeout, speedtest.UrlTestStandard_Handshake)
		if err == nil {
			results = append(results, strconv.Itoa(int(ms)))
		} else {
			results = append(results, err.Error())
		}
	}
	if mode&UdpTest != 0 {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Millisecond)
		defer cancel()

		start := time.Now()
		pc, err := libbox.DialContext(ctx, i, "udp", "8.8.8.8:53")
		if err == nil {
			defer pc.Close()
			_ = pc.SetDeadline(time.Now().Add(time.Duration(timeout) * time.Millisecond))
			dnsQuery := []byte{
				0x00, 0x00, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x03, 'w', 'w', 'w', 0x06, 'g', 'o', 'o', 'g', 'l', 'e', 0x03, 'c', 'o', 'm', 0x00,
				0x00, 0x01, 0x00, 0x01,
			}
			_, err = pc.Write(dnsQuery)
			if err == nil {
				var buf [512]byte
				_, err = pc.Read(buf[:])
			}
		}
		if err == nil {
			results = append(results, fmt.Sprint(time.Since(start).Milliseconds()))
		} else {
			results = append(results, err.Error())
		}
	}
	if mode&SpeedTest != 0 {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(speedTimeout))
		defer cancel()

		var n int64
		req, err := http.NewRequestWithContext(ctx, "GET", speedUrl, nil)
		start := time.Now()
		if err == nil {
			var resp *http.Response
			resp, err = httpClient.Do(req)
			if err == nil {
				defer resp.Body.Close()
				n, err = io.Copy(io.Discard, resp.Body)
			}
		}
		if err == nil {
			duration := math.Max(time.Since(start).Seconds(), 0.000001)
			results = append(results, fmt.Sprintf("%.2f", float64(n)/duration/MiB))
		} else {
			results = append(results, err.Error())
		}
	}
	if mode&IpTest != 0 {
		var in_ip, out_ip, country string

		if host, _, err := net.SplitHostPort(address); err != nil {
			in_ip = err.Error()
		} else if ipaddr, err := net.ResolveIPAddr("ip", host); err != nil {
			in_ip = err.Error()
		} else {
			in_ip = ipaddr.String()
		}

		resp, err := httpClient.Get("http://ip-api.com/json/")
		if err == nil {
			defer resp.Body.Close()
			var data map[string]any
			json.NewDecoder(resp.Body).Decode(&data)
			out_ip = data["query"].(string)
			country = data["countryCode"].(string)
		} else {
			out_ip = err.Error()
		}

		if len(country) == 2 {
			// country code to flag emoji
			// 'A' = 0x41 → 0x1F1E6
			country = string([]rune{
				rune(country[0]-'A') + 0x1F1E6,
				rune(country[1]-'A') + 0x1F1E6,
			})
		}

		results = append(results, in_ip+" → "+out_ip, country)
	}
	return C.CString(strings.Join(results, "\n"))
}

//export BoxStats
func BoxStats() *C.char {
	if mainInstance != nil {
		return C.CString(mainInstance.QueryStats2JSON())
	}
	return nil
}
