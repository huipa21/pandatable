package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strings"
	"testing"
	"time"
)

func dbFixture(t *testing.T, port string, running bool) *dbInspection {
	t.Helper()
	var info dbInspection
	body := `{"State":{"Running":false},"Config":{"Image":"docker.io/library/postgres:18"},"HostConfig":{"PortBindings":{"5432/tcp":[{"HostIp":"127.0.0.1","HostPort":"` + port + `"}]}},"Mounts":[{"Type":"volume","Name":"pt-dev-pgdata","Destination":"/var/lib/postgresql"}]}`
	if err := json.Unmarshal([]byte(body), &info); err != nil {
		t.Fatal(err)
	}
	info.State.Running = running
	return &info
}

func TestDatabasePort(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_, occupied, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := databasePort(occupied)
	if err != nil || port == occupied || port == "" {
		t.Fatalf("occupied port not replaced: port=%s err=%v", port, err)
	}
	rebound, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", port))
	if err != nil {
		t.Fatalf("port probe retained listener: %v", err)
	}
	if err := rebound.Close(); err != nil {
		t.Fatal(err)
	}
	if got, err := databasePort(port); err != nil || got != port {
		t.Fatalf("free preferred port changed: port=%s err=%v", got, err)
	}
}

func TestDevDatabaseUp(t *testing.T) {
	for _, mode := range []string{"running", "stopped-free", "stopped-occupied", "absent", "start-race", "create-race", "exhausted", "unrelated-error", "unsafe-mount", "wrong-volume", "remove-error"} {
		t.Run(mode, func(t *testing.T) {
			listener, err := net.Listen("tcp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			_, originalPort, err := net.SplitHostPort(listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			if mode == "stopped-free" || mode == "start-race" || mode == "unrelated-error" || mode == "exhausted" {
				if err := listener.Close(); err != nil {
					t.Fatal(err)
				}
			}
			info := dbFixture(t, originalPort, mode == "running")
			if mode == "absent" || mode == "create-race" {
				info = nil
			} else if mode == "unsafe-mount" {
				info.Mounts[0].Type = "bind"
			} else if mode == "wrong-volume" {
				info.Mounts[0].Name = "someone-elses-data"
			}
			originalImage := "docker.io/library/postgres:18"
			if info != nil {
				info.Config.Image = postgresImage
				originalImage = info.Config.Image
			}
			var calls []string
			starts, removes := 0, 0
			db := devDatabase{command: func(_ context.Context, args ...string) (string, error) {
				calls = append(calls, strings.Join(args, " "))
				switch args[0] {
				case "container":
					if info == nil {
						return "", nil
					}
					return dbContainer, nil
				case "inspect":
					body, err := json.Marshal(info)
					return string(body), err
				case "rm":
					if mode == "remove-error" {
						return "permission denied", errors.New("remove failed")
					}
					if len(args) != 2 || info == nil || info.canRecreate() != nil {
						t.Fatalf("unsafe container removal: %v", args)
					}
					removes++
					info = nil
					return "", nil
				case "run", "start":
					starts++
					if args[0] == "run" {
						if args[len(args)-1] != originalImage {
							t.Fatalf("recreation changed PostgreSQL image: %v", args)
						}
						published := ""
						for index, arg := range args {
							if arg == "-p" {
								published = args[index+1]
							}
						}
						if !strings.HasPrefix(published, "127.0.0.1:") || !strings.HasSuffix(published, ":5432") ||
							!strings.Contains(strings.Join(args, " "), "-v "+dbVolume+":/var/lib/postgresql") {
							t.Fatalf("wrong publication or missing persistent volume: %v", args)
						}
						port := strings.Split(published, ":")[1]
						info = dbFixture(t, port, false)
						info.Config.Image = originalImage
					}
					if mode == "unrelated-error" {
						return "image unavailable", errors.New("run failed")
					}
					if mode == "exhausted" || (starts == 1 && (mode == "start-race" || mode == "create-race")) {
						return "bind: address already in use", errors.New("exit status 125")
					}
					info.State.Running = true
					return "", nil
				case "exec":
					return "accepting connections", nil
				default:
					t.Fatalf("unexpected command: %v", args)
					return "", errors.New("unexpected command")
				}
			}}
			dsn, err := db.up(context.Background())
			wantErr := mode == "exhausted" || mode == "unrelated-error" || mode == "unsafe-mount" || mode == "wrong-volume" || mode == "remove-error"
			if (err != nil) != wantErr {
				t.Fatalf("dsn=%s err=%v calls=%v", dsn, err, calls)
			}
			if wantErr {
				if dsn != "" || (mode == "exhausted" && starts != 3) || (mode == "unrelated-error" && starts != 1) ||
					((mode == "unsafe-mount" || mode == "wrong-volume" || mode == "remove-error") && (starts != 0 || removes != 0)) {
					t.Fatalf("unsafe recovery or unbounded retry: starts=%d removes=%d dsn=%s", starts, removes, dsn)
				}
				return
			}
			parsed, err := url.Parse(dsn)
			if err != nil {
				t.Fatal(err)
			}
			port, err := info.port()
			if err != nil || parsed.Hostname() != "127.0.0.1" || parsed.Port() != port {
				t.Fatalf("DSN disagrees with engine binding: dsn=%s port=%s err=%v", dsn, port, err)
			}
			if mode == "running" && (starts != 0 || removes != 0 || port != originalPort) {
				t.Fatal("running DB was modified instead of reused")
			}
			if mode == "stopped-free" && (starts != 1 || removes != 0 || port != originalPort) {
				t.Fatal("free stopped DB was recreated unnecessarily")
			}
			if mode == "stopped-occupied" && (removes != 1 || port == originalPort) {
				t.Fatal("occupied port not recovered by volume-preserving recreation")
			}
			if (mode == "start-race" || mode == "create-race") && (starts != 2 || removes != 1) {
				t.Fatal("bind race did not retry once")
			}
		})
	}
}

func TestDevDatabaseErrors(t *testing.T) {
	for _, mode := range []string{"list", "inspect", "decode", "readiness"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			info := dbFixture(t, "55555", true)
			db := devDatabase{command: func(_ context.Context, args ...string) (string, error) {
				switch args[0] {
				case "container":
					if mode == "list" {
						return "permission denied", errors.New("list failed")
					}
					return dbContainer, nil
				case "inspect":
					if mode == "inspect" {
						return "permission denied", errors.New("inspect failed")
					}
					if mode == "decode" {
						return "bad JSON", nil
					}
					body, err := json.Marshal(info)
					return string(body), err
				default:
					return "not ready", errors.New("readiness failed")
				}
			}}
			dsn, err := db.up(ctx)
			if err == nil || dsn != "" {
				t.Fatalf("failure swallowed: dsn=%q err=%v", dsn, err)
			}
			if mode == "readiness" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("readiness deadline lost: %v", err)
			}
		})
	}
}

func TestDevDatabaseRejectsInvalidBinding(t *testing.T) {
	for _, port := range []string{"0", "-1", "65536", "bad", ""} {
		if _, err := dbFixture(t, port, true).port(); err == nil {
			t.Fatalf("invalid port accepted: %q", port)
		}
	}
	info := dbFixture(t, "5432", true)
	info.HostConfig.PortBindings["5432/tcp"][0].HostIP = "0.0.0.0"
	if _, err := info.port(); err == nil {
		t.Fatal("non-loopback binding accepted")
	}
}
