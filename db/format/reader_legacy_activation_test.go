package format

import (
	"bufio"
	"strings"
	"testing"
)

func TestReadQueuedTaskActivationV4(t *testing.T) {
	// LambdaCore-latest.db at MongooseMoo/lambdamoo-db-py commit
	// 15bdc121a7c4bfbaa45e1a5cc7d7ed92b427e345, blob
	// 261a7a73486c0fa83e503fed18e4ff026152d286: queued activation and
	// the following environment header. Managed Toast imports the LF dump.
	r := bufio.NewReader(strings.NewReader("0\n-111\n2 -7 -8 2 -9 2 88 -10 8\nNo\nMore\nParse\nInfos\ndisfunc\ndisfunc\n20 variables\n"))
	database := &Database{Version: 4}
	task := &QueuedTask{}
	if err := database.readQueuedTaskActivation(r, task); err != nil {
		t.Fatal(err)
	}
	if task.This != 2 || task.Player != 2 || task.Programmer != 2 || task.VerbLoc != 88 || task.Verb != "disfunc" {
		t.Fatalf("activation = %+v", task)
	}
	if next, err := readLine(r); err != nil || next != "20 variables" {
		t.Fatalf("next record = %q, %v; want environment header", next, err)
	}
}
