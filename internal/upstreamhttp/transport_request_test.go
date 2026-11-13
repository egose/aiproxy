package upstreamhttp

import (
	"context"
	"crypto/tls"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestProfileStreamingMultipartAndRedirectIsolation(t *testing.T) {
	cert, roots := transportTestCertificate(t)
	for _, alpn := range []string{"http/1.1", "h2"} {
		for _, route := range []string{"direct", "http-connect", "https-connect"} {
			t.Run(alpn+"/"+route, func(t *testing.T) {
				var destinations atomic.Int32
				destination, _, _, _ := transportTestServer(t, cert, tls.VersionTLS13, alpn, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					destinations.Add(1)
					t.Error("credential-bearing request crossed redirect origin")
				}))
				server, _, _, _ := transportTestServer(t, cert, tls.VersionTLS13, alpn, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					if req.Header.Get("Authorization") != "Bearer synthetic-provider" {
						t.Error("provider credential was not preserved")
					}
					switch req.URL.Path {
					case "/external":
						http.Redirect(w, req, destination.URL, http.StatusTemporaryRedirect)
						return
					case "/same":
						http.Redirect(w, req, "/finished", http.StatusTemporaryRedirect)
						return
					case "/finished":
						body, _ := io.ReadAll(req.Body)
						if req.Method != http.MethodPost || string(body) != "same-origin-body" {
							t.Error("same-origin redirect changed body/method")
						}
					case "/multipart":
						reader, err := req.MultipartReader()
						if err != nil {
							t.Error(err)
							return
						}
						part, err := reader.NextPart()
						if err != nil {
							t.Error(err)
							return
						}
						body, err := io.ReadAll(part)
						if err != nil || part.FormName() != "file" || string(body) != strings.Repeat("synthetic audio bytes", 1024) {
							t.Error("streaming multipart data changed")
						}
					}
					_, _ = io.WriteString(w, "ok")
				}))
				var proxy *url.URL
				if route != "direct" {
					proxy, _, _ = transportTestProxy(t, cert, route == "https-connect")
				}
				client, _ := transportTestClient(t, transportTestProfile(t, "chrome"), roots, proxy, time.Second)
				for _, target := range []string{"/external", "/same", "/multipart"} {
					var body io.Reader = strings.NewReader("same-origin-body")
					var writerDone <-chan error
					contentType := ""
					if target == "/multipart" {
						reader, writer := io.Pipe()
						multipartWriter := multipart.NewWriter(writer)
						contentType = multipartWriter.FormDataContentType()
						body = reader
						finished := make(chan error, 1)
						writerDone = finished
						go func() {
							part, err := multipartWriter.CreateFormFile("file", "synthetic.wav")
							if err == nil {
								_, err = io.WriteString(part, strings.Repeat("synthetic audio bytes", 1024))
							}
							if err == nil {
								err = multipartWriter.Close()
							}
							_ = writer.CloseWithError(err)
							finished <- err
						}()
					}
					ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
					req, _ := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+target, body)
					req.Header.Set("Authorization", "Bearer synthetic-provider")
					if contentType != "" {
						req.Header.Set("Content-Type", contentType)
					}
					resp, err := Do(client, req)
					if target == "/external" {
						if err == nil || !strings.Contains(err.Error(), ErrRedirectOrigin.Error()) || destinations.Load() != 0 {
							t.Fatalf("cross-origin redirect was not blocked: %v", err)
						}
					} else {
						if err != nil {
							cancel()
							t.Fatal(err)
						}
						_, _ = io.Copy(io.Discard, resp.Body)
						_ = resp.Body.Close()
					}
					cancel()
					if writerDone != nil {
						if err := <-writerDone; err != nil {
							t.Fatal(err)
						}
					}
				}
			})
		}
	}
}
