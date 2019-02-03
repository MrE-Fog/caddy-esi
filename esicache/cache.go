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

package esicache

import (
	"context"
	"sync"
	"time"

	"github.com/corestoreio/errors"
)

// Cacher used to cache the response of a micro service as found in the src
// attribute of an ESI tag. But the Cacher gets only involved if the additional
// attribute ttl has been set for each ESI tag. A Cacher must be thread safe.
type Cacher interface {
	Set(key string, value []byte, expiration time.Duration) error
	Get(key string) ([]byte, error)
}

// NewCacher creates a new cache service object and its connection as defined by
// its URL.
func NewCacher(url string) (Cacher, error) {

	return nil, nil
}

// Caches gets set during config reading and implements Cacher interface
type Caches []Cacher

// Set writes to the cache service
func (c Caches) Set(key string, value []byte, expiration time.Duration) error {
	// write to all
	return nil
}

// Get fetches from the cache service
func (c Caches) Get(key string) ([]byte, error) {
	// race condition which cache returns first
	return nil, nil
}

// MainRegistry global cache registry
var MainRegistry = &registry{
	caches: make(map[string]Caches),
}

type registry struct {
	mu sync.RWMutex
	// caches for each scope (string key), aka. each path in the Caddyfile, we
	// might have different but same named caches or even no caches.
	caches map[string]Caches
}

func (r *registry) Get(ctx context.Context, scope, alias, key string) error {
	return errors.New("TODO IMPLEMENT")
}

// Register registers a new key-value service. Scope refers to the URL provided
// in the Caddyfile after the `esi` keyword. URL represents the destination to
// Redis or Memcache etc
func (r *registry) Register(scope, url string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	c, err := NewCacher(url)
	if err != nil {
		return errors.Wrapf(err, "[esikv] NewCacher URL %q", url)
	}

	if _, ok := r.caches[scope]; !ok {
		r.caches[scope] = make(Caches, 0, 2)
	}
	r.caches[scope] = append(r.caches[scope], c)

	return nil
}

// Len returns number of registered caches.
func (r *registry) Len(scope string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.caches[scope])
}

// Clear removes all cache service objects
func (r *registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.caches = make(map[string]Caches)
}
