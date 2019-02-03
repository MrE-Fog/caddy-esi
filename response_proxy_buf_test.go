// Copyright 2015-present, Cyrill @ Schumacher.fm and the CoreStore contributors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package caddyesi

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Check if types have the interfaces implemented.
var _ http.CloseNotifier = &bufferedFancyWriter{}
var _ http.Flusher = &bufferedFancyWriter{}
var _ http.Hijacker = &bufferedFancyWriter{}
var _ http.Pusher = &bufferedFancyWriter{}
var _ io.ReaderFrom = &bufferedFancyWriter{}
var _ http.Flusher = &bufferedFlushWriter{}

func TestWrapBuffered(t *testing.T) {
	t.Parallel()

	wOrg := httptest.NewRecorder()
	buf := new(bytes.Buffer)
	wb := responseWrapBuffer(buf, wOrg)
	data := []byte(`Commander Data encrypts the computer with a fractal algorithm to protect it from the Borgs.`)
	n, err := wb.Write(data)
	assert.NoError(t, err)
	assert.Exactly(t, len(data), n)
	assert.Exactly(t, 0, wOrg.Body.Len())
	assert.Exactly(t, len(data), buf.Len())

	wb.Header().Set("Content-Length", "321")

	wb.TriggerRealWrite(3000)
	wb.WriteHeader(http.StatusTeapot)
	if _, err := wb.Write(buf.Bytes()); err != nil {
		t.Fatal(err)
	}

	assert.Exactly(t, 91, wOrg.Body.Len())
	assert.Exactly(t, http.StatusTeapot, wOrg.Code, "HTTP Status Code")
	assert.Exactly(t, `3321`, wOrg.Header().Get("Content-Length"))
}
