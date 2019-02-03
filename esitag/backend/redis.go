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

// +build esiall esiredis

package backend

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/corestoreio/caddy-esi/esitag"
	"github.com/corestoreio/errors"
	"github.com/gomodule/redigo/redis"
)

// https://github.com/gomodule/redigo/issues/207 <-- context to be added to the package: declined.
// TODO: instead of getting a single key via GET, we must use MGET to retrieve
// multiple keys within one connection to Redis. During page parsing we know
// ahead of all possible Redis keys!

func init() {
	esitag.RegisterResourceHandlerFactory("redis", NewRedis)
}

type esiRedis struct {
	isCancellable bool
	url           string
	pool          *redis.Pool
}

// NewRedis provides, for now, a basic implementation for simple key fetching.
func NewRedis(opt *esitag.ResourceOptions) (esitag.ResourceHandler, error) {
	addr, pw, params, err := opt.ParseNoSQLURL()
	if err != nil {
		return nil, errors.NotValid.Newf("[backend] Redis error parsing URL %q => %s", opt.URL, err)
	}

	maxActive, err := strconv.Atoi(params.Get("max_active"))
	if err != nil {
		return nil, errors.NotValid.Newf("[backend] NewRedis.ParseNoSQLURL. Parameter max_active not valid in  %q", opt.URL)
	}
	maxIdle, err := strconv.Atoi(params.Get("max_idle"))
	if err != nil {
		return nil, errors.NotValid.Newf("[backend] NewRedis.ParseNoSQLURL. Parameter max_idle not valid in  %q", opt.URL)
	}
	idleTimeout, err := time.ParseDuration(params.Get("idle_timeout"))
	if err != nil {
		return nil, errors.NotValid.Newf("[backend] NewRedis.ParseNoSQLURL. Parameter idle_timeout not valid in  %q", opt.URL)
	}

	r := &esiRedis{
		isCancellable: params.Get("cancellable") == "1",
		url:           opt.URL,
		pool: &redis.Pool{
			MaxActive:   maxActive,
			MaxIdle:     maxIdle,
			IdleTimeout: idleTimeout,
			Dial: func() (redis.Conn, error) {
				c, err := redis.Dial("tcp", addr)
				if err != nil {
					return nil, errors.Wrap(err, "[backend] Redis Dial failed")
				}
				if pw != "" {
					if _, err := c.Do("AUTH", pw); err != nil {
						c.Close()
						return nil, errors.Wrap(err, "[backend] Redis AUTH failed")
					}
				}
				if _, err := c.Do("SELECT", params.Get("db")); err != nil {
					c.Close()
					return nil, errors.Wrap(err, "[backend] Redis DB select failed")
				}
				return c, nil
			},
		},
	}

	if params.Get("lazy") == "1" {
		return r, nil
	}

	conn := r.pool.Get()
	defer conn.Close()

	pong, err := redis.String(conn.Do("PING"))
	if err != nil && err != redis.ErrNil {
		return nil, errors.Fatal.Newf("[backend] Redis Ping failed: %s", err)
	}
	if pong != "PONG" {
		return nil, errors.Fatal.Newf("[backend] Redis Ping not Pong: %#v", pong)
	}

	return r, nil
}

// Closes closes the resource when Caddy restarts or reloads. If supported
// by the resource.
func (er *esiRedis) Close() error {
	return errors.Wrapf(er.pool.Close(), "[backend] Redis Close. URI %q", er.url)
}

// DoRequest returns a value from the field Key in the args argument. Header is
// not supported. Request cancellation through a timeout (when the client
// request gets cancelled) is supported.
func (er *esiRedis) DoRequest(args *esitag.ResourceArgs) (_ http.Header, _ []byte, err error) {
	if er.isCancellable {
		// 50000	     28794 ns/op	    1026 B/op	      33 allocs/op
		return er.doRequestCancel(args)
	}
	// 50000	     25071 ns/op	     529 B/op	      25 allocs/op
	return er.doRequest(args)
}

func (er *esiRedis) doRequest(args *esitag.ResourceArgs) (_ http.Header, _ []byte, err error) {
	if err := args.ValidateWithKey(); err != nil {
		return nil, nil, errors.Wrap(err, "[backend] doRequest.ValidateWithKey")
	}

	conn := er.pool.Get()
	defer conn.Close()

	value, err := redis.Bytes(conn.Do("GET", args.Tag.Key))
	if err == redis.ErrNil {
		return nil, nil, errors.NotFound.Newf("[backend] URL %q: Key %q not found", er.url, args.Tag.Key)
	}
	if err != nil {
		return nil, nil, errors.Wrapf(err, "[backend] Redis.Get %q => %q", er.url, args.Tag.Key)
	}

	if mbs := int(args.Tag.MaxBodySize); len(value) > mbs && mbs > 0 {
		value = value[:mbs]
	}

	return nil, value, err
}

// DoRequest returns a value from the field Key in the args argument. Header is
// not supported. Request cancellation through a timeout (when the client
// request gets cancelled) is supported.
func (er *esiRedis) doRequestCancel(args *esitag.ResourceArgs) (_ http.Header, _ []byte, err error) {
	if err := args.ValidateWithKey(); err != nil {
		return nil, nil, errors.Wrap(err, "[backend] doRequest.ValidateWithKey")
	}

	// See git history for a version without context.WithTimeout. A bit faster and less allocs.
	ctx, cancel := context.WithTimeout(args.ExternalReq.Context(), args.Tag.Timeout)
	defer cancel()

	content := make(chan []byte)
	retErr := make(chan error)
	go func() {

		conn := er.pool.Get()
		defer conn.Close()

		value, err := redis.Bytes(conn.Do("GET", args.Tag.Key))
		if err == redis.ErrNil {
			retErr <- errors.NotFound.Newf("[backend] URL %q: Key %q not found", er.url, args.Tag.Key)
			return
		}
		if err != nil {
			retErr <- errors.Wrapf(err, "[backend] Redis.Get %q => %q", er.url, args.Tag.Key)
			return
		}

		if mbs := int(args.Tag.MaxBodySize); len(value) > mbs && mbs > 0 {
			value = value[:mbs]
		}
		content <- value
	}()

	var value []byte
	select {
	case <-ctx.Done():
		err = errors.Wrapf(ctx.Err(), "[backend] Redits Get Context cancelled. Previous possible error: %+v", retErr)
	case value = <-content:
	case err = <-retErr:
	}
	return nil, value, err
}
