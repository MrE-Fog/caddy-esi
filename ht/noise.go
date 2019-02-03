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

package main

import (
	"fmt"
	"os"
	"time"

	"github.com/vdobler/ht/ht"
)

var noisyCounter chan int

func init() {
	if noNoise := os.Getenv("ESI_DISABLE_NOISE"); noNoise != "" {
		println("[ht/main] Background noise requests disabled: ", noNoise)
		return
	}

	noisyCounter = make(chan int) // must be blocking
	// generator for incremented integers to be race free
	go func() {
		var i int
		for {
			noisyCounter <- i
			i++
		}
	}()

	// <Background noise>
	go func() {
		for c := time.Tick(1 * time.Millisecond); ; <-c {
			go func() {
				// each test needs ~2ms
				t := noisyRequests()
				if err := t.Run(); err != nil {
					panic(fmt.Sprintf("Test %q\nError: %s", t.Name, err))
				}
			}()
		}
	}()
	// </Background noise>
}

func noisyRequests() (t *ht.Test) {
	reqID := <-noisyCounter
	t = &ht.Test{
		Name:    fmt.Sprintf("Noisy Request %d", reqID),
		Request: makeRequestGET(fmt.Sprintf("ms_cart_tiny.html?id=%d", reqID)),
		Checks: makeChecklist200(
			&ht.Body{
				Contains: "price-excluding-tax", // see integration.sh
				Count:    2,
			},
		),
	}
	return
}
