package server

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/command"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/trace"
	"github.com/MongooseMoo/barn/types"
)

func TestTraceLoginKeepsPayloadOutOfHostOutput(t *testing.T) {
	for _, filters := range [][]string{nil, {"do_login*", "echo", "fail"}} {
		t.Run(strings.Join(filters, ","), func(t *testing.T) {
			var output bytes.Buffer
			var logs bytes.Buffer
			previousLogger := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
			t.Cleanup(func() { slog.SetDefault(previousLogger) })
			trace.Init(true, filters, &output)
			t.Cleanup(func() { trace.Init(false, nil, nil) })
			s, conn, store := loginTestSetup(t, []string{
				`if (length(args) == 3)`,
				`  notify(player, "NOTIFY_SECRET_209");`,
				`  return this:echo(args[3]);`,
				`endif`,
				`return #-1;`,
			})
			addTestVerb(store, 10, "echo", `return args[1];`)
			store.AddVerb(10, dbstore.NewVerb("fail", []string{"fail"}, 2,
				dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
				dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
				[]string{`raise(E_INVARG, args[1], args);`}))
			transport := &recordingTransport{readLine: make(chan string), remote: "REMOTE_SECRET_209:7777"}
			conn.transport = transport
			line := "connect USER_SECRET_209 PASSWORD_SECRET_209"
			if player, err := s.callDoLoginCommand(conn, line); err != nil || player >= 0 {
				t.Fatalf("direct login result: player=%d err=%v", player, err)
			}
			s.processInput(command.InputEvent{ConnID: conn.ID, Player: types.ObjID(-conn.ID), Line: line})
			if conn.IsLoggedIn() {
				t.Fatal("string login result unexpectedly logged in")
			}
			if got := transport.writtenLines(); !strings.Contains(strings.Join(got, "\n"), "NOTIFY_SECRET_209") {
				t.Fatalf("notification delivery changed: %v", got)
			}
			result := s.runtime.CallVerb(10, "echo", []types.Value{types.NewStr("RETURN_SECRET_209")}, 2)
			if result.Flow != types.FlowReturn || result.Val.Str() != "RETURN_SECRET_209" {
				t.Fatal("tracing changed return value")
			}
			waif := types.NewWaif(10, 2).SetProperty("private", types.NewStr("WAIF_SECRET_209"))
			for _, value := range []types.Value{
				types.NewList([]types.Value{types.NewList([]types.Value{types.NewStr("LIST_SECRET_209")})}),
				types.NewMap([][2]types.Value{{types.NewStr("MAP_KEY_SECRET_209"), types.NewList([]types.Value{types.NewStr("MAP_VALUE_SECRET_209"), waif})}}),
				waif,
			} {
				result = s.runtime.CallVerb(10, "echo", []types.Value{value}, 2)
				if result.Flow != types.FlowReturn || !result.Val.Equal(value) {
					t.Fatal("tracing changed container result")
				}
			}
			result = s.runtime.CallVerb(10, "fail", []types.Value{types.NewStr("ERROR_SECRET_209"), types.NewStr("ERROR_ARG_SECRET_209")}, 2)
			if result.Flow != types.FlowException || result.Error != types.E_INVARG {
				t.Fatalf("exception flow=%d error=%s type=%s", result.Flow, result.Error, result.Val.Type())
			}
			// Drive the real connection lifecycle with an EOF transport. The
			// processor drains the welcome and disconnect events before returning.
			close(transport.readLine)
			transport.closed = true
			s.Start()
			s.HandleConnection(conn)
			s.Stop()
			got := output.String() + logs.String()
			for _, secret := range []string{"USER_SECRET_209", "PASSWORD_SECRET_209", "RETURN_SECRET_209", "NOTIFY_SECRET_209", "REMOTE_SECRET_209", "ERROR_SECRET_209", "ERROR_ARG_SECRET_209", "LIST_SECRET_209", "MAP_KEY_SECRET_209", "MAP_VALUE_SECRET_209", "WAIF_SECRET_209", "<waif"} {
				if strings.Contains(got, secret) {
					t.Errorf("trace disclosed %s: %s", secret, got)
				}
			}
			for _, event := range []string{`CALL #10:"do_login_command" argc=3`, `RETURN #10:"echo" type=STR`, `RETURN #10:"echo" type=LIST`, `RETURN #10:"echo" type=MAP`, `RETURN #10:"echo" type=WAIF`, `EXCEPTION #10:"fail" E_INVARG`, "NOTIFY", "CONN NEW", "CONN DISCONNECT"} {
				if !strings.Contains(got, event) {
					t.Errorf("missing structural event %q: %s", event, got)
				}
			}
		})
	}
}
