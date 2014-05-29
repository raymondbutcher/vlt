// VLT: Varnish Load Tester
//
// This program takes the output from varnishlog on one server, and makes
// identical HTTP requests to another. It uses the same headers as the
// original requests. It does not support POST data.
//
//
// Requirements:
//		Varnish
//
// Usage:
// 		vlt my-host.com
//
// VLT reads output from:
// 	varnishlog -c -o -u -i RxRequest,RxURL,RxProtocol,RxHeader,ReqEnd
// 	-c      Include log entries which result from communication with a client.
// 	-o      Group log entries by request ID.
// 	-o      Ignored for compatibility with earlier versions.
// 	-u      Unbuffered output.
// 	-i tag  Include log entries with the specified tag.
//
// Which looks something like:
//   270 SessionOpen  c 109.77.56.26 50315 209.49.145.8:80
//   270 RxRequest    c GET
//   270 RxURL        c /comments/best-dogs/
//   270 RxProtocol   c HTTP/1.1
//   270 RxHeader     c Host: www.dogs.com
//   270 RxHeader     c Referer: http://www.dogs.com/best-dogs/
//   270 RxHeader     c X-Requested-With: XMLHttpRequest
//   270 RxHeader     c Accept-Encoding: gzip, deflate
//   270 RxHeader     c Accept: */*
//   270 RxHeader     c Accept-Language: en-us
//   270 RxHeader     c Cookie: dogman-85=160797435.90310.0000
//   270 RxHeader     c Connection: keep-alive
//   270 RxHeader     c User-Agent: Mozilla/5.0 (iPhone; CPU iPhone OS 7_1_1 like Mac OS X) AppleWebKit/537.51.2 (KHTML, like Gecko) Version/7.0 Mobile/11D201 Safari/9537.53
//   270 ReqEnd       c 1933456148 1401232289.094973087 1401232289.121248960 0.000027418 0.026240110 0.000035763

package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

// The key/value string positions in varnishlog output.
const (
	LOG_KEY_START   int = 6
	LOG_KEY_END     int = 19
	LOG_VALUE_START int = 21
)

type Request struct {
	Method   string
	Path     string
	Protocol string
	Headers  *http.Header
}

func NewRequest() *Request {
	return &Request{
		Headers: &http.Header{},
	}
}

func (req *Request) AddHeader(str string) {
	header := strings.SplitN(str, ":", 2)
	key := strings.TrimSpace(header[0])
	value := strings.TrimSpace(header[1])
	req.Headers.Add(key, value)
}

func (req *Request) GetHost() string {
	return strings.TrimSpace(req.Headers.Get("Host"))
}

func (req *Request) GetURL(target_host string) (*url.URL, error) {

	req_url, err := url.Parse(req.Path)
	if err != nil {
		return nil, err
	}

	if req.Protocol[0:4] == "HTTP" {
		if len(req.Protocol) >= 5 && req.Protocol[4:5] == "S" {
			req_url.Scheme = "https"
		} else {
			req_url.Scheme = "http"
		}
	} else {
		err := fmt.Errorf("Unknown scheme: %s\n", req.Protocol)
		return nil, err
	}

	req_url.Host = target_host

	return req_url, nil
}

func (req *Request) SendRequest(target_host string) {

	req_url, err := req.GetURL(target_host)
	if err != nil {
		log.Print(err)
		return
	}

	major, minor, ok := http.ParseHTTPVersion(req.Protocol)
	if !ok {
		log.Printf("Unknown protocol: %s\n", req.Protocol)
		return
	}

	original_host := req.GetHost()

	http_req := &http.Request{
		Method:     req.Method,
		URL:        req_url,
		Proto:      req.Protocol,
		ProtoMajor: major,
		ProtoMinor: minor,
		Header:     *req.Headers,
		Host:       original_host,
	}

	start := time.Now()

	// Use the lower level Transport.RoundTrip
	// to avoid http.Client's redirect handling.
	http_resp, err := http.DefaultTransport.RoundTrip(http_req)

	elapsed := time.Since(start) / time.Millisecond

	// Ensure that negative numbers are not displayed. This can happen in virtual
	// machines. There is no monononic clock functionality in Go at this time, so
	// for now I will just ensure that everything shows as 1 millisecond or more.
	if elapsed < 1 {
		elapsed = 1
	}

	if err == nil {
		req_url.Host = original_host
		log.Printf("[%dms] [%d] %s %s\n", elapsed, http_resp.StatusCode, req.Method, req_url)
	} else {
		log.Printf("[%dms] [%s] %s %s\n", elapsed, err, req.Method, req_url)
	}

}

func main() {

	if len(os.Args) != 2 {
		fmt.Print("Usage: vlt <host>\n")
		os.Exit(1)
	}
	target_host := strings.TrimRight(strings.TrimSpace(os.Args[1]), "/")

	req := NewRequest()

	// Run varnishlog and parse the output. Each HTTP request is made up of
	// multiple lines of output, starting with RxRequest and finishing with
	// ReqEnd. When a full HTTP request has been prepared, it gets sent to
	// the target server in a goroutine.
	log_stdout := varnishlog()
	log_scanner := bufio.NewScanner(log_stdout)
	for log_scanner.Scan() {
		line := log_scanner.Text()
		if len(line) > LOG_VALUE_START {
			key := strings.TrimSpace(line[LOG_KEY_START:LOG_KEY_END])
			if key == "RxRequest" {
				req = NewRequest()
				req.Method = line[LOG_VALUE_START:]
			} else if key == "RxURL" {
				req.Path = line[LOG_VALUE_START:]
			} else if key == "RxProtocol" {
				req.Protocol = line[LOG_VALUE_START:]
			} else if key == "RxHeader" {
				req.AddHeader(line[LOG_VALUE_START:])
			} else if key == "ReqEnd" {
				go req.SendRequest(target_host)
			}
		}
	}
	if err := log_scanner.Err(); err != nil {
		log.Fatal(err)
	}
}

func varnishlog() io.ReadCloser {
	// Runs a varnishlog process and returns a stdout pipe for reading.

	cmd := exec.Command("varnishlog", "-c", "-o", "-u", "-i", "RxRequest,RxURL,RxProtocol,RxHeader,ReqEnd")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		log.Fatal(err)
	}

	return stdout
}
