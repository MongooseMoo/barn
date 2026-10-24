package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRuntimeSummaryFiltersTasksAndNeverPrintsPayload(t *testing.T) {
	path := writeLog(t,
		`{"msg":"runtime phase","task_id":7,"phase":"vm_attempt","elapsed_ns":1000000,"ticks":40,"secret":"do not print"}`,
		`{"msg":"runtime phase","task_id":7,"phase":"retry_waste","elapsed_ns":1000000,"ticks":40}`,
		`{"msg":"runtime phase","task_id":8,"phase":"vm_attempt","elapsed_ns":9000000,"ticks":900}`,
		`{"msg":"threaded lifecycle","task_id":7,"stage":"start"}`,
		`{"msg":"threaded lifecycle","task_id":7,"stage":"complete","accepted":false}`,
		tracebackRecord,
		`{"msg":"runtime phase","task_id":7,"phase":"vm_attempt","elapsed_ns":-1,"ticks":3}`,
		`{"msg":"cut off`)
	got, err := readRuntimeSummary(path, 7)
	if err != nil {
		t.Fatal(err)
	}
	if got.PhaseRecords != 2 || got.Started != 1 || got.Completed != 1 || got.Rejected != 1 || got.Errors != 1 || got.InvalidRecords != 2 {
		t.Fatalf("accounting: %+v", got)
	}
	if len(got.Phases) != 2 || got.Phases[0].Phase != "retry_waste" || got.Phases[1].Ticks != 40 {
		t.Fatalf("phase ordering/filter: %+v", got.Phases)
	}
	var out bytes.Buffer
	printRuntimeSummary(&out, got)
	if strings.Contains(out.String(), "do not print") || strings.Contains(out.String(), "x =") || !strings.Contains(out.String(), "wall") {
		t.Fatalf("payload leakage or ambiguous clock: %s", out.String())
	}
}

func TestRuntimeSummaryReportsMissingMeasurements(t *testing.T) {
	got, err := readRuntimeSummary(writeLog(t, infoRecord), -1)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	printRuntimeSummary(&out, got)
	if !strings.Contains(out.String(), "not measured") {
		t.Fatalf("missing data presented as zero cost: %s", out.String())
	}
}

func TestTerminalExceptionSummaryFiltersAndBoundsVocabulary(t *testing.T) {
	path := writeLog(t,
		`{"msg":"terminal task exception","task_id":7,"error":"E_MAXREC","reason":"seconds_budget","intrinsic_eval":false,"committed":true,"value":"private-sentinel"}`,
		`{"msg":"terminal task exception","task_id":7,"error":"E_MAXREC","reason":"seconds_budget","intrinsic_eval":false,"committed":true}`,
		`{"msg":"terminal task exception","task_id":7,"error":"E_MAXREC","reason":"unknown","intrinsic_eval":true,"committed":false}`,
		`{"msg":"terminal task exception","task_id":8,"error":"E_INVARG","reason":"unknown","intrinsic_eval":false,"committed":true}`,
		`{"msg":"terminal task exception","task_id":7,"error":"private-error","reason":"unknown","intrinsic_eval":false}`,
		`{"msg":"terminal task exception","task_id":7,"error":"E_MAXREC","reason":"private-reason","intrinsic_eval":false}`,
		`{"msg":"terminal task exception","task_id":7,"error":"E_INVARG","reason":"unknown"}`,
		`{"msg":"terminal task exception","error":"E_INVARG","reason":"unknown","intrinsic_eval":false}`,
		`{"msg":"terminal task exception","task_id":7,"error":"E_NONE","reason":"unknown","intrinsic_eval":false,"committed":true}`,
		`{"msg":"terminal task exception","task_id":7,"error":"E_INVARG","reason":"seconds_budget","intrinsic_eval":false,"committed":true}`)
	got, err := readRuntimeSummary(path, 7)
	if err != nil {
		t.Fatal(err)
	}
	if got.ExceptionRecords != 4 || got.InvalidRecords != 5 || len(got.Exceptions) != 3 {
		t.Fatalf("bad census: %+v", got)
	}
	if got.Exceptions[0].Count != 2 || got.Exceptions[0].Reason != "seconds_budget" || got.Exceptions[1].Count != 1 || !got.Exceptions[1].IntrinsicEval {
		t.Fatalf("wrong ordered rows: %+v", got.Exceptions)
	}
	var out bytes.Buffer
	printRuntimeSummary(&out, got)
	if strings.Contains(out.String(), "private-") || !strings.Contains(out.String(), "reason=seconds_budget") || !strings.Contains(out.String(), "originating task IDs") {
		t.Fatalf("unsafe or missing terminal summary: %s", out.String())
	}
	if !strings.Contains(out.String(), "Runtime phases not measured") {
		t.Fatalf("exceptions fabricated phase measurements: %s", out.String())
	}
}
