package config

import (
	"errors"
	"flag"
	"io"
	"os"
)

// HealthcheckListen deliberately validates only the probe's configuration.
func HealthcheckListen(args []string) (string, error) {
	return healthcheckListen(args, os.LookupEnv)
}

func healthcheckListen(args []string, lookup func(string) (string, bool)) (string, error) {
	listen := ":8080"
	if value, ok := lookup("PT_LISTEN"); ok {
		listen = value
	}
	flags := flag.NewFlagSet("healthcheck", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&listen, "listen", listen, "HTTP listen address to probe")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return "", flag.ErrHelp
		}
		return "", UsageError{message: "invalid healthcheck flags (use --help for options)"}
	}
	if len(flags.Args()) != 0 {
		return "", UsageError{message: "healthcheck does not accept positional arguments"}
	}
	return validateListen(listen)
}
