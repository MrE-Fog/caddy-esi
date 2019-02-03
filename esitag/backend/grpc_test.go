// Copyright 2015-present, Cyrill @ Schumacher.fm and the CoreStore contributors
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may not
// use this file except in compliance with the License. You may obtain a copy of
// the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations under
// the License.

// +build esiall esigrpc

package backend_test

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/corestoreio/caddy-esi/esitag"
	"github.com/corestoreio/caddy-esi/esitag/backend"
	"github.com/corestoreio/caddy-esi/esitesting"
	"github.com/corestoreio/errors"
	"github.com/corestoreio/log"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/grpclog"
)

const (
	// also stored in server file
	serverListenAddr = "grpc://127.0.0.1:50049"
)

// useTestLogging if false creates a noop logger for grpc and if true uses the
// testing.T logging but triggers a race but only then when you run the test with -count=>1
const useTestLogging = false

func init() {
	if !useTestLogging {
		grpclog.SetLogger(grpcLogTestWrap{})
	}
}

func TestNewGRPCClient(t *testing.T) {
	t.Parallel()
	// enable only for debugging the tests because this global package logger
	// triggers a race :-(. Bad architecture in the gRPC package ...
	if useTestLogging {
		grpclog.SetLogger(grpcLogTestWrap{tb: t})
	}

	cmd := esitesting.StartProcess("go", "run", "grpc_server_main.go")
	go cmd.Wait()            // waits forever until killed
	defer cmd.Process.Kill() // kills the go process but not the main started server
	// when subtests which uses grpcInsecureClient run in parallel then you have
	// to comment this out because you don't know when the sub tests finishes
	// and the GRPC server gets killed before the tests finishes.
	defer esitesting.KillZombieProcess("grpc_server_main")

	t.Run("Error in ParseNoSQLURL", func(t *testing.T) {
		t.Parallel()
		cl, err := backend.NewGRPCClient(esitag.NewResourceOptions("grpc://127::01:1:90000"))
		if err == nil {
			t.Error("Missing required error")
		}
		if !errors.NotValid.Match(err) {
			t.Errorf("error should have behaviour NotValid: %+v", err)
		}
		if cl != nil {
			t.Errorf("cl should be nil, but got: %#v", cl)
		}
	})

	t.Run("Error in timeout in query string", func(t *testing.T) {
		t.Parallel()
		cl, err := backend.NewGRPCClient(esitag.NewResourceOptions(serverListenAddr + "?timeout="))
		if err == nil {
			t.Error("Missing required error")
		}
		// tb.Log(err)
		if !errors.NotValid.Match(err) {
			t.Errorf("error should have behaviour NotValid: %+v", err)
		}
		if cl != nil {
			t.Errorf("cl should be nil, but got: %#v", cl)
		}
	})

	t.Run("Error because ca_file not found", func(t *testing.T) {
		t.Parallel()
		cl, err := backend.NewGRPCClient(esitag.NewResourceOptions(
			serverListenAddr + "?timeout=10s&tls=1&ca_file=testdata/non_existent.pem",
		))
		if err == nil {
			t.Error("Missing required error")
		}
		if !errors.Fatal.Match(err) {
			t.Errorf("error should have behaviour Fatal: %+v", err)
		}
		if cl != nil {
			t.Errorf("cl should be nil, but got: %#v", cl)
		}
	})

	t.Run("Error server unreachable", func(t *testing.T) {
		t.Parallel()
		// limit timeout to 1s otherwise we'll maybe wait too long, after 1sec
		// the context gets cancelled.
		cl, err := backend.NewGRPCClient(esitag.NewResourceOptions(
			"grpc://127.0.0.1:81049?timeout=1s",
		))
		if err == nil {
			t.Error("Missing required error")
		}
		//tb.Log(err)
		if !errors.Fatal.Match(err) {
			t.Errorf("error should have behaviour Fatal: %+v", err)
		}
		if cl != nil {
			t.Errorf("cl should be nil, but got: %#v", cl)
		}
	})

	grpcInsecureClient, err := backend.NewGRPCClient(esitag.NewResourceOptions(
		// 60s deadline to wait until server is up and running. GRPC will do
		// a reconnection. Race detector slows down the program.
		serverListenAddr + "?timeout=60s",
	))
	if err != nil {
		t.Fatalf("Whooops: %+v", err)
	}

	t.Run("Connect insecure and retrieve HTML data", func(t *testing.T) {
		const key = `should be echoed back into the content response`

		const iterations = 10
		var wg sync.WaitGroup
		wg.Add(iterations)
		for i := 0; i < iterations; i++ {
			go func(wg *sync.WaitGroup) { // food for the race detector
				defer wg.Done()

				rfa := esitag.NewResourceArgs(
					getExternalReqWithExtendedHeaders(),
					"grpcShoppingCart1",
					esitag.Config{
						Timeout:     5 * time.Second,
						MaxBodySize: 3333,
						Key:         key,
						Log:         log.BlackHole{},
					},
				)

				hdr, content, err := grpcInsecureClient.DoRequest(rfa)
				if err != nil {
					t.Fatalf("Woops: %+v", err)
				}
				if hdr != nil {
					t.Errorf("Header should be nil because not yet supported: %#v", hdr)
				}

				assert.Contains(t, string(content), key)
				assert.Contains(t, string(content), `<p>Arg URL: grpcShoppingCart1</p>`)

			}(&wg)
		}
		wg.Wait()

	})

	t.Run("Connect insecure and retrieve error from server", func(t *testing.T) {

		rfa := esitag.NewResourceArgs(
			getExternalReqWithExtendedHeaders(),
			"grpcShoppingCart2",
			esitag.Config{
				Timeout:     5 * time.Second,
				MaxBodySize: 3333,
				Key:         "word error in the key triggers an error on the server",
				Log:         log.BlackHole{},
			},
		)

		hdr, content, err := grpcInsecureClient.DoRequest(rfa)
		if hdr != nil {
			t.Errorf("Header should be nil because not yet supported: %#v", hdr)
		}
		if content != nil {
			t.Errorf("Content should be nil: %q", content)
		}
		assert.Contains(t, err.Error(), `[grpc_server] Interrupted. Detected word error in "word error in the key triggers an error on the server" for URL "grpcShoppingCart2"`)
	})

	t.Run("POST request gets its body echoed back from the gRPC server", func(t *testing.T) {

		rfa := esitag.NewResourceArgs(
			getExternalReqWithExtendedHeaders(),
			"grpcShoppingCart2",
			esitag.Config{
				ForwardPostData: true,
				Timeout:         5 * time.Second,
				MaxBodySize:     1e5,
				Key:             "too_large",
				Log:             log.BlackHole{},
			},
		)
		rfa.ExternalReq.Method = "POST"
		rfa.ExternalReq.Body = ioutil.NopCloser(bytes.NewBufferString("This string exceeds not the 1e5 bytes defined in the resource arguments type."))

		hdr, content, err := grpcInsecureClient.DoRequest(rfa)
		assert.NoError(t, err)
		if hdr != nil {
			t.Errorf("Header should be nil because not yet supported: %#v", hdr)
		}
		assert.Contains(t, string(content), `<p>BodyEcho: This string exceeds not the 1e5 bytes defined in the resource arguments type.</p>`)
	})
}

// BenchmarkNewGRPCClient_Parallel/Insecure-4         	   10000	    173541 ns/op	   69317 B/op	      66 allocs/op
func BenchmarkNewGRPCClient_Parallel(b *testing.B) {

	// This parent benchmark function runs only once as soon as there is another
	// sub-benchmark.
	cmd := esitesting.StartProcess("go", "run", "grpc_server_main.go")
	go cmd.Wait()            // waits forever until killed
	defer cmd.Process.Kill() // kills the go process but not the main started server
	defer esitesting.KillZombieProcess("grpc_server_main")

	if useTestLogging {
		grpclog.SetLogger(grpcLogTestWrap{tb: b})
	}
	// Full integration benchmark test which starts a GRPC server and uses TCP
	// to query it on the localhost.

	const lenCartExampleHTML = 21601

	b.Run("Insecure", func(b *testing.B) {

		grpcInsecureClient, err := backend.NewGRPCClient(esitag.NewResourceOptions(
			// 20s deadline to wait until server is up and running. GRPC will do
			// a reconnection.
			serverListenAddr + "?timeout=20s",
		))
		if err != nil {
			b.Fatalf("Whooops: %+v", err)
		}

		rfa := esitag.NewResourceArgs(
			getExternalReqWithExtendedHeaders(),
			"http://totally-uninteresting.what",
			esitag.Config{
				Key:         `cart_example.html`,
				Timeout:     time.Second,
				MaxBodySize: 22001,
				Log:         log.BlackHole{},
			},
		)

		b.ResetTimer()
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			var content []byte
			var hdr http.Header
			var err error

			for pb.Next() {
				hdr, content, err = grpcInsecureClient.DoRequest(rfa)
				if err != nil {
					b.Fatalf("%+v", err)
				}
				if hdr != nil {
					b.Fatal("Header should be nil")
				}
				if len(content) != lenCartExampleHTML {
					b.Fatalf("Want %d\nHave %d", lenCartExampleHTML, len(content))
				}
			}
		})
	})
}

type grpcLogTestWrap struct {
	// tb, if nil we have a noop logger which is needed to avoid race conditions
	// and disable output logging to stdout in package grpclog. This global
	// package logger shows a sub-optimal architecture.
	tb testing.TB
}

func (lw grpcLogTestWrap) Fatal(args ...interface{}) {
	if lw.tb != nil {
		lw.tb.Fatal(args...)
	}
}
func (lw grpcLogTestWrap) Fatalf(format string, args ...interface{}) {
	if lw.tb != nil {
		lw.tb.Fatalf(format, args...)
	}
}
func (lw grpcLogTestWrap) Fatalln(args ...interface{}) {
	if lw.tb != nil {
		lw.tb.Fatal(args...)
	}
}
func (lw grpcLogTestWrap) Print(args ...interface{}) {
	if lw.tb != nil {
		lw.tb.Log(args...)
	}
}
func (lw grpcLogTestWrap) Printf(format string, args ...interface{}) {
	if lw.tb != nil {
		lw.tb.Logf(format, args...)
	}
}
func (lw grpcLogTestWrap) Println(args ...interface{}) {
	if lw.tb != nil {
		lw.tb.Log(args...)
	}
}
