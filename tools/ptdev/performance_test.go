package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPerformanceOptionsAndSeedManifest(t *testing.T) {
	opts, err := parseArgs([]string{"e2e", "--perf"})
	if err != nil || !opts.perf || len(opts.e2eArgs) != 0 {
		t.Fatal("performance workflow is not explicit and fixed")
	}
	if _, err := parseArgs([]string{"e2e", "--perf", "--", "--retries=9"}); err == nil {
		t.Fatal("benchmark accepted overriding browser arguments")
	}
	valid := `{"database":"pt_seed_owned","tableId":"table","viewId":"view","stage":"complete","requestedRows":200000,"committedRows":200000,"complete":true}`
	for _, invalid := range []string{"", `{}`, valid + "\n" + valid, strings.Replace(valid, `"complete":true`, `"complete":false`, 1),
		strings.Replace(valid, `"committedRows":200000`, `"committedRows":199999`, 1)} {
		if _, err := seedManifestFromOutput(invalid); err == nil {
			t.Fatal("accepted incomplete/ambiguous seed evidence")
		}
	}
	if _, err := seedManifestFromOutput("compose provider notice\n" + valid); err != nil {
		t.Fatal(err)
	}
	if _, err := seedManifestFromOutput("\x1b[4mcompose provider notice\n\n\x1b[0m" + valid); err != nil {
		t.Fatal("Compose ANSI prefix hid the valid seed result:", err)
	}
	full := strings.TrimSuffix(valid, "}") + `,"workspaceId":"workspace","bookId":"book","fieldIds":{"name":"field"},"attemptedRows":200000}`
	manifest, err := seedManifestFromOutput(full)
	if err != nil || manifest.WorkspaceID != "workspace" || manifest.BookID != "book" ||
		manifest.FieldIDs["name"] != "field" || manifest.AttemptedRows != performanceRows {
		t.Fatal("stdlib-only CLI wire decoding lost fixture ownership metadata")
	}
}

func TestPerformanceHostEvidence(t *testing.T) {
	for _, input := range []string{
		`{"host":{"cpus":8,"memTotal":2147483648,"os":"linux","kernel":"wsl","distribution":{"distribution":"fedora","version":"43"}}}`,
		`{"NCPU":2,"MemTotal":4294967296,"OperatingSystem":"Ubuntu 24.04","KernelVersion":"linux"}`,
	} {
		data, err := performanceHost(input)
		if err != nil {
			t.Fatal(err)
		}
		var result struct {
			CPUs     int    `json:"cpus"`
			RAMBytes uint64 `json:"ramBytes"`
			Boundary string `json:"boundary"`
		}
		if err := json.Unmarshal(data, &result); err != nil || result.CPUs == 0 || result.RAMBytes == 0 || result.Boundary == "" {
			t.Fatal("host evidence lost measured resources/boundary")
		}
	}
	for _, invalid := range []string{"not json", `{}`, `{"NCPU":2,"MemTotal":0}`} {
		if _, err := performanceHost(invalid); err == nil {
			t.Fatal("host observation silently fabricated")
		}
	}
}
