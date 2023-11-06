// Copyright 2023 Talhuang<talhuang1231@gmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

// Package apiserver does all the work necessary to create a APIServer.
package apiserver

import (
	"github.com/skeleton1231/gotal/internal/apiserver/config"
	"github.com/skeleton1231/gotal/internal/apiserver/options"
	"github.com/skeleton1231/gotal/pkg/app"
)

const serverName = "APISERVER"
const commandDesc = `Description`

// NewApp creates an App object with default parameters.
func NewApp(basename string) *app.App {

	opts := options.NewOptions()
	application := app.NewApp(serverName,
		basename,
		app.WithOptions(opts),
		app.WithDescription(serverName+commandDesc),
		app.WithDefaultValidArgs(),
		app.WithRunFunc(run(opts)),
	)

	return application
}

func run(opts *options.Options) app.RunFunc {
	return func(basename string) error {

		cfg, err := config.CreateConfigFromOptions(opts)
		if err != nil {
			return err
		}

		return Run(cfg)
	}
}
