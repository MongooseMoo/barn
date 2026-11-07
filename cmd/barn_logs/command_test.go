package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestCommandSummaryCorrelatesInputAndPhases(t *testing.T) {
	path := writeLog(t,
		`{"msg":"player command","command_id":11,"kind":"command","stage":"classified","elapsed_ns":100000000}`,
		`{"msg":"player command","command_id":11,"kind":"command","stage":"first_output","elapsed_ns":200000000,"payload":"private"}`,
		`{"msg":"player command","command_id":12,"kind":"command","stage":"finished","elapsed_ns":500,"response_written":false}`,
		`{"msg":"runtime phase","command_id":11,"task_id":7,"phase":"slice","elapsed_ns":900,"ticks":1}`,
		`{"msg":"runtime phase","command_id":12,"task_id":8,"phase":"slice","elapsed_ns":600,"ticks":1}`,
		`{"msg":"other","level":"ERROR"}`)
	summary, err := readRuntimeSummary(path, -1, 11)
	if err != nil {
		t.Fatal(err)
	}
	if summary.CommandRecords != 2 || summary.PhaseRecords != 1 || summary.Errors != 1 {
		t.Fatalf("%+v", summary)
	}
	var out bytes.Buffer
	printRuntimeSummary(&out, summary)
	if strings.Contains(out.String(), "private") || !strings.Contains(out.String(), "200ms") {
		t.Fatal(out.String())
	}
	all, err := readRuntimeSummary(path, -1, -1)
	if err != nil {
		t.Fatal(err)
	}
	if all.CommandsWithoutWrite != 1 {
		t.Fatalf("missing silent command: %+v", all)
	}
}
