package main

import "testing"

func TestParseArgs(t *testing.T) {
	for _, test := range []struct {
		args    []string
		command string
		wantErr bool
	}{
		{nil, "help", false},
		{[]string{"--help"}, "help", false},
		{[]string{"db", "up"}, "db up", false},
		{[]string{"db", "reset", "--yes"}, "db reset", false},
		{[]string{"build", "--linux"}, "build", false},
		{[]string{"test", "--no-db", "-run", "TestEngine", "-v", "-count=1"}, "test", false},
		{[]string{"web", "build"}, "web build", false},
		{[]string{"image", "save"}, "image save", false},
		{[]string{"image", "save", "--with-postgres", "--version=1.2.3+abc"}, "image save", false},
		{[]string{"image", "build", "--version", "1.2.3"}, "image build", false},
		{[]string{"image", "build", "--with-postgres"}, "image build", true},
		{[]string{"image", "save", "--version"}, "image save", true},
		{[]string{"image", "save", "--version=../bad"}, "image save", true},
		{[]string{"deploy", "--host", "ops@server", "--yes"}, "deploy", false},
		{[]string{"deploy", "--yes"}, "deploy", true},
		{[]string{"deploy", "--host", "server"}, "deploy", true},
		{[]string{"deploy", "--host", "--yes"}, "deploy", true},
		{[]string{"deploy", "--host=-bad", "--yes"}, "deploy", true},
		{[]string{"db"}, "db", true},
		{[]string{"db", "oops"}, "db oops", true},
		{[]string{"oops"}, "oops", true},
		{[]string{"build", "--yes"}, "build", true},
		{[]string{"test", "-run"}, "test", true},
		{[]string{"test", "-count=oops"}, "test", true},
		{[]string{"test", "-v=oops"}, "test", true},
		{[]string{"version", "extra"}, "version", true},
	} {
		got, err := parseArgs(test.args)
		if got.command != test.command || (err != nil) != test.wantErr {
			t.Errorf("parseArgs(%v) = %+v, %v", test.args, got, err)
		}
	}
	got, err := parseArgs([]string{"deploy", "--host", "ops@server", "--dir", "/opt/data with spaces", "--yes", "--with-postgres", "--version", "1.2.3"})
	if err != nil || got.host != "ops@server" || got.remoteDir != "/opt/data with spaces" || !got.withPostgres || !got.yes || got.releaseVersion != "1.2.3" {
		t.Fatalf("deploy flags: %+v, %v", got, err)
	}
	got, err = parseArgs([]string{"test", "--no-db", "-run=Test", "-count", "2", "-v=false"})
	if err != nil || !got.noDB || len(got.testArgs) != 3 || got.testArgs[1] != "-count=2" {
		t.Fatalf("test flag forwarding: %+v, %v", got, err)
	}
	got, err = parseArgs([]string{"build", "--linux"})
	if err != nil || !got.linux {
		t.Fatalf("linux flag: %+v, %v", got, err)
	}
	got, err = parseArgs([]string{"db", "reset", "--yes"})
	if err != nil || !got.yes {
		t.Fatalf("reset flag: %+v, %v", got, err)
	}
}
