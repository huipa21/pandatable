package main

import (
	"fmt"
	"strconv"
	"strings"
)

type options struct {
	command        string
	linux          bool
	yes            bool
	noDB           bool
	testArgs       []string
	withPostgres   bool
	releaseVersion string
	host           string
	remoteDir      string
}

func parseArgs(args []string) (options, error) {
	result := options{command: "help"}
	if len(args) == 0 {
		return result, nil
	}
	result.command = args[0]
	args = args[1:]
	if result.command == "--help" || result.command == "-h" {
		result.command = "help"
	}
	switch result.command {
	case "db", "web", "image":
		if len(args) == 0 {
			return result, usageError{"missing sub-command for " + result.command}
		}
		result.command += " " + args[0]
		args = args[1:]
	}
	switch result.command {
	case "help", "version", "engine", "db up", "db down", "db reset", "db status", "db psql", "build", "lint", "test", "check", "dev", "web build", "e2e", "image build", "image save", "deploy":
	default:
		return result, usageError{"unknown command: " + result.command}
	}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case result.command == "build" && arg == "--linux":
			result.linux = true
		case result.command == "db reset" && arg == "--yes":
			result.yes = true
		case result.command == "test" && arg == "--no-db":
			result.noDB = true
		case (result.command == "image save" || result.command == "deploy") && arg == "--with-postgres":
			result.withPostgres = true
		case result.command == "deploy" && arg == "--yes":
			result.yes = true
		case result.command == "image build" || result.command == "image save" || result.command == "deploy":
			name, value, hasValue := strings.Cut(arg, "=")
			if name != "--version" && !(result.command == "deploy" && (name == "--host" || name == "--dir")) {
				return result, usageError{"unknown image/deploy flag: " + name}
			}
			if !hasValue {
				index++
				if index == len(args) {
					return result, usageError{"missing value for " + name}
				}
				value = args[index]
			}
			if value == "" || strings.HasPrefix(value, "--") {
				return result, usageError{"missing value for " + name}
			}
			switch name {
			case "--version":
				if _, err := imageTag(value); err != nil {
					return result, err
				}
				result.releaseVersion = value
			case "--host":
				result.host = value
			case "--dir":
				result.remoteDir = value
			}
		case result.command == "test":
			name, value, hasValue := strings.Cut(arg, "=")
			if name != "-run" && name != "-v" && name != "-count" {
				return result, usageError{"unknown test flag: " + arg}
			}
			if name != "-v" && !hasValue {
				index++
				if index == len(args) {
					return result, usageError{"missing value for " + name}
				}
				value = args[index]
				hasValue = true
			}
			if name == "-count" {
				count, err := strconv.Atoi(value)
				if err != nil || count < 0 {
					return result, usageError{"-count requires a non-negative integer"}
				}
			}
			if name == "-v" && hasValue {
				if _, err := strconv.ParseBool(value); err != nil {
					return result, usageError{"-v requires a boolean"}
				}
			}
			if hasValue {
				arg = name + "=" + value
			}
			result.testArgs = append(result.testArgs, arg)
		default:
			return result, usageError{fmt.Sprintf("unexpected argument %q for %s", arg, result.command)}
		}
	}
	if result.command == "deploy" {
		if result.remoteDir == "" {
			result.remoteDir = "/opt/pandatable"
		}
		if err := validateDestination(result.host, result.remoteDir); err != nil {
			return result, err
		}
		if !result.yes {
			return result, usageError{"deploy requires --yes to approve remote image loads, .env version update and Compose start"}
		}
	}
	return result, nil
}
