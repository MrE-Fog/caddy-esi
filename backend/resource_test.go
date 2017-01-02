package backend_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/SchumacherFM/caddyesi/backend"
	"github.com/corestoreio/errors"
	"github.com/stretchr/testify/assert"
)

var _ fmt.Stringer = (*backend.Resource)(nil)

func TestNewResource(t *testing.T) {
	t.Run("URL", func(t *testing.T) {
		r, err := backend.NewResource(0, "http://cart.service")
		if err != nil {
			t.Fatalf("%+v", err)
		}
		assert.Exactly(t, `http://cart.service`, r.String())
	})

	t.Run("URL is an alias", func(t *testing.T) {
		r, err := backend.NewResource(0, "awsRedisCartService")
		if err != nil {
			t.Fatalf("%+v", err)
		}
		assert.Exactly(t, `awsRedisCartService`, r.String())
	})

	t.Run("URL scheme not found", func(t *testing.T) {
		r, err := backend.NewResource(0, "ftp://cart.service")
		assert.Nil(t, r)
		assert.True(t, errors.IsNotSupported(err), "%+v", err)
	})

	t.Run("URL Template", func(t *testing.T) {
		r, err := backend.NewResource(0, "http://cart.service?product={{ .r.Header.Get }}")
		if err != nil {
			t.Fatalf("%+v", err)
		}
		assert.Exactly(t, "http://cart.service?product={{ .r.Header.Get }} Template: resource_tpl", r.String())
	})

	t.Run("URL Template throws fatal error", func(t *testing.T) {
		r, err := backend.NewResource(0, "http://cart.service?product={{ r.Header.Get }}")
		assert.Nil(t, r)
		assert.True(t, errors.IsFatal(err), "%+v", err)
	})
}

func TestResource_CircuitBreaker(t *testing.T) {
	t.Parallel()

	r, err := backend.NewResource(1, "http://to/a/location")
	if err != nil {
		t.Fatalf("%+v", err)
	}
	state, lastFailure := r.CBState()
	assert.Exactly(t, backend.CBStateClosed, state, "CBStateClosed")
	assert.Exactly(t, time.Unix(1, 0), lastFailure, "lastFailure")

	assert.Exactly(t, uint64(0), r.CBFailures(), "CBFailures()")
	fail := r.CBRecordFailure()
	assert.True(t, fail > 0, "Timestamp greater 0")

	fail = r.CBRecordFailure()
	assert.True(t, fail > 0, "Timestamp greater 0")

	state, lastFailure = r.CBState()
	assert.Exactly(t, backend.CBStateClosed, state, "CBStateClosed")
	assert.True(t, lastFailure.UnixNano() > fail, "lastFailure greater than recorded failure")

	assert.Exactly(t, uint64(2), r.CBFailures(), "CBFailures()")

	var i uint64
	for ; i < backend.CBMaxFailures; i++ {
		r.CBRecordFailure()
	}
	assert.Exactly(t, 14, int(r.CBFailures()), "CBFailures()")

	state, lastFailure = r.CBState()
	assert.Exactly(t, backend.CBStateOpen, state, "CBStateOpen")
	assert.True(t, lastFailure.UnixNano() > fail, "lastFailure greater than recorded failure")
}

func TestRequestFuncArgs_Validate(t *testing.T) {
	t.Run("URL", func(t *testing.T) {
		rfa := backend.RequestFuncArgs{}
		err := rfa.Validate()
		assert.True(t, errors.IsEmpty(err), "%+v", err)
		assert.Contains(t, err.Error(), `URL value`)
	})
	t.Run("ExternalReq", func(t *testing.T) {
		rfa := backend.RequestFuncArgs{
			URL: "http://www",
		}
		err := rfa.Validate()
		assert.True(t, errors.IsEmpty(err), "%+v", err)
		assert.Contains(t, err.Error(), `ExternalReq value`)
	})
	t.Run("timeout", func(t *testing.T) {
		rfa := backend.RequestFuncArgs{
			URL:         "http://www",
			ExternalReq: httptest.NewRequest("GET", "/", nil),
		}
		err := rfa.Validate()
		assert.True(t, errors.IsEmpty(err), "%+v", err)
		assert.Contains(t, err.Error(), `timeout value`)
	})
	t.Run("maxBodySize", func(t *testing.T) {
		rfa := backend.RequestFuncArgs{
			URL:         "http://www",
			ExternalReq: httptest.NewRequest("GET", "/", nil),
			Timeout:     time.Second,
		}
		err := rfa.Validate()
		assert.True(t, errors.IsEmpty(err), "%+v", err)
		assert.Contains(t, err.Error(), `maxBodySize value`)
	})
	t.Run("Correct", func(t *testing.T) {
		rfa := backend.RequestFuncArgs{
			URL:         "http://www",
			ExternalReq: httptest.NewRequest("GET", "/", nil),
			Timeout:     time.Second,
			MaxBodySize: 5,
		}
		err := rfa.Validate()
		assert.NoError(t, err, "%+v", err)
	})
}

func TestRequestFuncArgs_MaxBodySizeHumanized(t *testing.T) {
	rfa := backend.RequestFuncArgs{
		MaxBodySize: 123456789,
	}
	assert.Exactly(t, `124 MB`, rfa.MaxBodySizeHumanized())
}

func getExternalReqWithExtendedHeaders() *http.Request {
	req := httptest.NewRequest("GET", "https://caddyserver.com/any/path", nil)
	req.Header = http.Header{
		"Host":                      []string{"www.example.com"},
		"Connection":                []string{"keep-alive"},
		"Pragma":                    []string{"no-cache"},
		"Cache-Control":             []string{"no-cache"},
		"Upgrade-Insecure-Requests": []string{"1"},
		"User-Agent":                []string{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10)"},
		"Accept":                    []string{"text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8"},
		"DNT":                       []string{"1"},
		"Referer":                   []string{"https://www.example.com/"},
		"Accept-Encoding":           []string{"gzip, deflate, sdch, br"},
		"Avail-Dictionary":          []string{"lhdx6rYE"},
		"Accept-Language":           []string{"en-US,en;q=0.8"},
		"Cookie":                    []string{"x-wl-uid=1vnTVF5WyZIe5Fymf2a4H+pFPyJa4wxNmzCKdImj1UqQPV5ecUs2sm46vDbGJUI+sE=", "session-token=AIo5Vf+c/GhoTRWq4V; JSESSIONID=58B7C7A24731R869B75D142E970CEAD4; csm-hit=D5P2DBNF895ZDJTCTEQ7+s-D5P2DBNF895ZDJTCTEQ7|1483297885458; session-id-time=2082754801l"},
	}
	return req
}

var resourceRespWithExtendedHeaders = http.Header{
	"Server":                    []string{"Server"},
	"Date":                      []string{"Mon, 02 Jan 2017 08:58:08 GMT"},
	"Content-Type":              []string{"text/html;charset=UTF-8"},
	"Transfer-Encoding":         []string{"chunked"},
	"Connection":                []string{"keep-alive"},
	"Strict-Transport-Security": []string{"max-age=47474747; includeSubDomains; preload"},
	"x-dmz-id-1":                []string{"XBXAV6DKR823M418TZ8Y"},
	"X-Frame-Options":           []string{"SAMEORIGIN"},
	"Cache-Control":             []string{"no-transform"},
	"Content-Encoding":          []string{"gzip"},
	"Vary":                      []string{"Accept-Encoding,Avail-Dictionary,User-Agent"},
	"Set-Cookie":                []string{"ubid-acbde=253-9771841-6878311; Domain=.example.com; Expires=Sun, 28-Dec-2036 08:58:08 GMT; Path=/"},
	"x-sdch-encode":             []string{"0"},
}

var benchmarkRequestFuncArgs_PrepareForwardHeaders []string

func BenchmarkRequestFuncArgs_PrepareForwardHeaders(b *testing.B) {

	rfa := &backend.RequestFuncArgs{
		ExternalReq:       getExternalReqWithExtendedHeaders(),
		ForwardHeadersAll: true,
	}

	b.Run("All", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkRequestFuncArgs_PrepareForwardHeaders = rfa.PrepareForwardHeaders()
		}
		if have, want := len(benchmarkRequestFuncArgs_PrepareForwardHeaders), 20; have != want {
			b.Fatalf("Have: %v Want: %v", have, want)
		}
	})

	b.Run("Two", func(b *testing.B) {
		rfa.ForwardHeadersAll = false
		rfa.ForwardHeaders = []string{"Cookie", "user-agent"}
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkRequestFuncArgs_PrepareForwardHeaders = rfa.PrepareForwardHeaders()
		}
		if have, want := len(benchmarkRequestFuncArgs_PrepareForwardHeaders), 4; have != want {
			b.Fatalf("Have: %v Want: %v", have, want)
		}
	})
}

var benchmarkRequestFuncArgs_PrepareReturnHeaders http.Header

func BenchmarkRequestFuncArgs_PrepareReturnHeaders(b *testing.B) {

	rfa := &backend.RequestFuncArgs{
		ExternalReq:      getExternalReqWithExtendedHeaders(),
		ReturnHeadersAll: true,
	}

	b.Run("All", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkRequestFuncArgs_PrepareReturnHeaders = rfa.PrepareReturnHeaders(resourceRespWithExtendedHeaders)
		}
		if have, want := len(benchmarkRequestFuncArgs_PrepareReturnHeaders), 6; have != want {
			b.Fatalf("Have: %v Want: %v", have, want)
		}
	})

	b.Run("Two", func(b *testing.B) {
		rfa.ReturnHeadersAll = false
		rfa.ReturnHeaders = []string{"Set-Cookie", "x-sdch-encode"}
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkRequestFuncArgs_PrepareReturnHeaders = rfa.PrepareReturnHeaders(resourceRespWithExtendedHeaders)
		}
		if have, want := len(benchmarkRequestFuncArgs_PrepareReturnHeaders), 2; have != want {
			b.Fatalf("Have: %v Want: %v", have, want)
		}
	})
}

func TestRequestFuncArgs_PrepareForwardHeaders(t *testing.T) {

	rfa := &backend.RequestFuncArgs{
		URL:         "http://whatever.anydomain/page.html",
		ExternalReq: getExternalReqWithExtendedHeaders(),
		Timeout:     time.Second,
		MaxBodySize: 15,
	}

	t.Run("ForwardHeaders none", func(t *testing.T) {
		assert.Exactly(t, []string{}, rfa.PrepareForwardHeaders())
	})

	t.Run("ForwardHeadersAll", func(t *testing.T) {
		rfa.ForwardHeadersAll = true
		rfa.ForwardHeaders = []string{"Cookie"} // gets ignored

		want := []string{
			"Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8",
			"Accept-Encoding", "gzip, deflate, sdch, br",
			"Accept-Language", "en-US,en;q=0.8",
			"Avail-Dictionary", "lhdx6rYE",
			"Cookie", "session-token=AIo5Vf+c/GhoTRWq4V; JSESSIONID=58B7C7A24731R869B75D142E970CEAD4; csm-hit=D5P2DBNF895ZDJTCTEQ7+s-D5P2DBNF895ZDJTCTEQ7|1483297885458; session-id-time=2082754801l",
			"Cookie", "x-wl-uid=1vnTVF5WyZIe5Fymf2a4H+pFPyJa4wxNmzCKdImj1UqQPV5ecUs2sm46vDbGJUI+sE=",
			"DNT", "1",
			"Referer", "https://www.example.com/",
			"Upgrade-Insecure-Requests", "1",
			"User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10)",
		}

		have := rfa.PrepareForwardHeaders()
		//t.Logf("%#v", have)

		if have, want := len(want), len(have); have != want {
			t.Fatalf("Differnt length of the lists Have: %v Want: %v", have, want)
		}

		for i := 0; i < len(want); i = i + 2 {
			for j := 0; j < len(have); j = j + 2 {
				// stupid slow comparison ... but ok for tests
				if want[i] == have[i] {
					assert.Exactly(t, want[i+1], have[i+1], "Key %q", want[i])
				}
			}
		}

	})

	t.Run("ForwardHeaders Some", func(t *testing.T) {
		rfa.ForwardHeadersAll = false
		rfa.ForwardHeaders = []string{"Cookie", "Pragma"} // Pragma gets dropped

		assert.Exactly(t,
			[]string{"Cookie", "x-wl-uid=1vnTVF5WyZIe5Fymf2a4H+pFPyJa4wxNmzCKdImj1UqQPV5ecUs2sm46vDbGJUI+sE=", "Cookie", "session-token=AIo5Vf+c/GhoTRWq4V; JSESSIONID=58B7C7A24731R869B75D142E970CEAD4; csm-hit=D5P2DBNF895ZDJTCTEQ7+s-D5P2DBNF895ZDJTCTEQ7|1483297885458; session-id-time=2082754801l"},
			rfa.PrepareForwardHeaders(),
		)
	})
}

func TestRequestFuncArgs_PrepareReturnHeaders(t *testing.T) {

	rfa := &backend.RequestFuncArgs{
		URL:         "http://whatever.anydomain/page.html",
		ExternalReq: getExternalReqWithExtendedHeaders(),
		Timeout:     time.Second,
		MaxBodySize: 15,
	}

	t.Run("ReturnHeaders none", func(t *testing.T) {
		assert.Nil(t, rfa.PrepareReturnHeaders(resourceRespWithExtendedHeaders))
	})

	t.Run("ReturnHeadersAll", func(t *testing.T) {
		rfa.ReturnHeadersAll = true
		rfa.ReturnHeaders = []string{"Set-Cookie"} // gets ignored

		want := http.Header{
			"Set-Cookie":      []string{"ubid-acbde=253-9771841-6878311; Domain=.example.com; Expires=Sun, 28-Dec-2036 08:58:08 GMT; Path=/"},
			"X-Dmz-Id-1":      []string{"XBXAV6DKR823M418TZ8Y"},
			"Server":          []string{"Server"},
			"Vary":            []string{"Accept-Encoding,Avail-Dictionary,User-Agent"},
			"X-Frame-Options": []string{"SAMEORIGIN"},
			"X-Sdch-Encode":   []string{"0"},
		}

		have := rfa.PrepareReturnHeaders(resourceRespWithExtendedHeaders)
		//t.Logf("%#v", have)
		assert.Exactly(t, want, have)
	})

	t.Run("ReturnHeaders Some", func(t *testing.T) {
		rfa.ReturnHeadersAll = false
		rfa.ReturnHeaders = []string{"Set-Cookie", "Connection"} // Connection gets dropped

		assert.Exactly(t,
			http.Header{"Set-Cookie": []string{"ubid-acbde=253-9771841-6878311; Domain=.example.com; Expires=Sun, 28-Dec-2036 08:58:08 GMT; Path=/"}},
			rfa.PrepareReturnHeaders(resourceRespWithExtendedHeaders),
		)
	})

}