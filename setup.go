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

package caddyesi

import (
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/corestoreio/caddy-esi/esicache"
	"github.com/corestoreio/caddy-esi/esitag"
	_ "github.com/corestoreio/caddy-esi/esitag/backend" // Let them register depending on the build tag
	"github.com/corestoreio/caddy-esi/helper"
	"github.com/corestoreio/errors"
	"github.com/corestoreio/log"
	"github.com/corestoreio/log/zapw"
	"github.com/dustin/go-humanize"
	"github.com/mholt/caddy"
	"github.com/mholt/caddy/caddyhttp/httpserver"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func init() {
	caddy.RegisterPlugin("esi", caddy.Plugin{
		ServerType: "http",
		Action:     PluginSetup,
	})
}

// PluginSetup used internally by Caddy to set up this middleware
func PluginSetup(c *caddy.Controller) error {
	pcs, err := configEsiParse(c)
	if err != nil {
		return errors.Wrap(err, "[caddyesi] Failed to parse configuration")
	}

	cfg := httpserver.GetConfig(c)

	mw := &Middleware{
		Root:        cfg.Root,
		FileSys:     http.Dir(cfg.Root),
		PathConfigs: pcs,
	}

	cfg.AddMiddleware(func(next httpserver.Handler) httpserver.Handler {
		mw.Next = next
		return mw
	})

	c.OnShutdown(func() error {
		return errors.Wrap(esitag.CloseAllResourceHandler(), "[caddyesi] OnShutdown")
	})
	c.OnRestart(func() error {
		// really necessary? investigate later
		for _, pc := range pcs {
			pc.purgeESICache()
		}
		return errors.Wrap(esitag.CloseAllResourceHandler(), "[caddyesi] OnRestart")
	})

	return nil
}

func configEsiParse(c *caddy.Controller) (PathConfigs, error) {
	pcs := make(PathConfigs, 0, 2)

	for c.Next() {
		pc := NewPathConfig()

		// Get the path scope
		args := c.RemainingArgs()
		switch len(args) {
		case 0:
			pc.Scope = "/"
		case 1:
			pc.Scope = args[0]
		default:
			return nil, c.ArgErr()
		}

		// Load any other configuration parameters
		for c.NextBlock() {
			if err := configLoadParams(c, pc); err != nil {
				return nil, errors.Wrap(err, "[caddyesi] Failed to load params")
			}
		}
		if err := setupLogger(pc); err != nil {
			return nil, errors.Wrap(err, "[caddyesi] Failed to setup Logger")
		}

		if pc.MaxBodySize == 0 {
			pc.MaxBodySize = DefaultMaxBodySize
		}
		if pc.Timeout == 0 {
			pc.Timeout = DefaultTimeOut
		}
		if len(pc.OnError) == 0 {
			pc.OnError = []byte(DefaultOnError)
		}

		pcs = append(pcs, pc)
	}
	return pcs, nil
}

// mocked out for testing
var osStdErr io.Writer = os.Stderr
var osStdOut io.Writer = os.Stdout

func setupLogger(pc *PathConfig) error {
	pc.Log = log.BlackHole{}
	const loggingDisabled zapcore.Level = -50

	var lvl = loggingDisabled
	switch pc.LogLevel {
	case "debug":
		lvl = zap.DebugLevel
	case "info":
		lvl = zap.InfoLevel
	case "fatal":
		lvl = zap.FatalLevel
	}

	if lvl == loggingDisabled {
		// logging disabled
		return nil
	}

	var w io.Writer
	switch pc.LogFile {
	case "stderr":
		w = osStdErr
	case "stdout":
		w = osStdOut
	case "":
		// logging disabled
		return nil
	default:
		var err error
		w, err = os.OpenFile(pc.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		// maybe handle file close on server restart or shutdown
		if err != nil {
			return errors.Fatal.Newf("[caddyesi] Failed to open file %q with error: %s", pc.LogFile, err)
		}
	}

	pc.Log = zapw.Wrap{
		Level: lvl,
		Zap: zap.New(
			zapcore.NewCore(
				zapcore.NewJSONEncoder(
					zapcore.EncoderConfig{
						MessageKey:     "msg",
						LevelKey:       "level",
						TimeKey:        "ts",
						NameKey:        "name",
						CallerKey:      "caller",
						StacktraceKey:  "stacktrace",
						EncodeLevel:    zapcore.LowercaseLevelEncoder,
						EncodeTime:     zapcore.ISO8601TimeEncoder,
						EncodeDuration: zapcore.NanosDurationEncoder,
					}),
				zapcore.AddSync(w),
				lvl,
			),
		),
	}

	return nil
}

func configLoadParams(c *caddy.Controller, pc *PathConfig) error {
	switch key := c.Val(); key {

	case "timeout":
		if !c.NextArg() {
			return errors.NotValid.Newf("[caddyesi] timeout: %s", c.ArgErr())
		}
		d, err := time.ParseDuration(c.Val())
		if err != nil {
			return errors.NotValid.Newf("[caddyesi] Invalid duration in timeout configuration: %q Error: %s", c.Val(), err)
		}
		pc.Timeout = d

	case "ttl":
		if !c.NextArg() {
			return errors.NotValid.Newf("[caddyesi] ttl: %s", c.ArgErr())
		}
		d, err := time.ParseDuration(c.Val())
		if err != nil {
			return errors.NotValid.Newf("[caddyesi] Invalid duration in ttl configuration: %q Error: %s", c.Val(), err)
		}
		pc.TTL = d

	case "max_body_size":
		if !c.NextArg() {
			return errors.NotValid.Newf("[caddyesi] max_body_size: %s", c.ArgErr())
		}
		d, err := humanize.ParseBytes(c.Val())
		if err != nil {
			return errors.NotValid.Newf("[caddyesi] Invalid max body size value configuration: %q Error: %s", c.Val(), err)
		}
		pc.MaxBodySize = d

	case "cache":
		if !c.NextArg() {
			return errors.NotValid.Newf("[caddyesi] cache: %s", c.ArgErr())
		}

		if err := esicache.MainRegistry.Register(pc.Scope, c.Val()); err != nil {
			return errors.Wrapf(err, "[caddyesi] esicache.MainRegistry.Register Key %q with URL: %q", key, c.Val())
		}

	case "page_id_source":
		if !c.NextArg() {
			return errors.NotValid.Newf("[caddyesi] page_id_source: %s", c.ArgErr())
		}
		pc.PageIDSource = helper.CommaListToSlice(c.Val())

	case "allowed_methods":
		if !c.NextArg() {
			return errors.NotValid.Newf("[caddyesi] allowed_methods: %s", c.ArgErr())
		}
		pc.AllowedMethods = helper.CommaListToSlice(strings.ToUpper(c.Val()))
	case "cmd_header_name":
		if !c.NextArg() {
			return errors.NotValid.Newf("[caddyesi] cmd_header_name: %s", c.ArgErr())
		}
		pc.CmdHeaderName = http.CanonicalHeaderKey(c.Val())
	case "on_error":
		if !c.NextArg() {
			return errors.NotValid.Newf("[caddyesi] allowed_methods: %s", c.ArgErr())
		}
		if err := pc.parseOnError(c.Val()); err != nil {
			return errors.Wrap(err, "[caddyesi] PathConfig.parseOnError")
		}
	case "log_file":
		if !c.NextArg() {
			return errors.NotValid.Newf("[caddyesi] log_file: %s", c.ArgErr())
		}
		pc.LogFile = c.Val()
	case "log_level":
		if !c.NextArg() {
			return errors.NotValid.Newf("[caddyesi] log_level: %s", c.ArgErr())
		}
		pc.LogLevel = strings.ToLower(c.Val())
	case "resources":
		if !c.NextArg() {
			return errors.NotValid.Newf("[caddyesi] resources: %s", c.ArgErr())
		}
		// c.Val() contains the file name or raw-content ;-)
		items, err := UnmarshalResourceItems(c.Val())
		if err != nil {
			return errors.Wrapf(err, "[caddyesi] Failed to unmarshal resource config %q", c.Val())
		}
		for _, item := range items {
			f, err := esitag.NewResourceHandler(esitag.NewResourceOptions(item.URL, item.Alias, item.Query))
			if err != nil {
				// may disclose passwords which are stored in the URL
				return errors.Wrapf(err, "[caddyesi] esikv Service init failed for URL %q in file %q", item.URL, c.Val())
			}
			esitag.RegisterResourceHandler(item.Alias, f)
		}
	default:
		c.NextArg()
		return errors.NotSupported.Newf("[caddyesi] Key %q with value %q not supported", key, c.Val())
	}

	return nil
}
