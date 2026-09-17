package engine

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// limitsDynamicStatement is features/limits_dynamic.yaml::setadd_checks_list_max_value_bytes verbatim.
const limitsDynamicStatement = `
had_catchable = 1;
try
  original_catchable = $server_options.max_concat_catchable;
except (E_PROPNF)
  had_catchable = 0;
  original_catchable = 0;
endtry
had_list_limit = 1;
try
  original_list_limit = $server_options.max_list_value_bytes;
except (E_PROPNF)
  had_list_limit = 0;
  original_list_limit = 0;
endtry
outcome = E_NONE;
cleanup_failed = 0;
try
  if (!had_catchable)
    add_property($server_options, "max_concat_catchable", 1, {player, "r"});
  else
    $server_options.max_concat_catchable = 1;
  endif
  if (!had_list_limit)
    add_property($server_options, "max_list_value_bytes", 1000000, {player, "r"});
  else
    $server_options.max_list_value_bytes = 1000000;
  endif
  n = 90;
  load_server_options();
  pad = value_bytes({1}) - value_bytes({});
  x = {};
  for i in [1..n]
    x = setadd(x, i);
  endfor
  size = value_bytes(x);
  $server_options.max_list_value_bytes = size + pad;
  load_server_options();
  try
    x = {};
    for i in [1..(n + 1)]
      x = setadd(x, i);
    endfor
    outcome = E_NONE;
  except error (E_QUOTA)
    outcome = error[1];
  endtry
finally
  try
    if (had_list_limit)
      $server_options.max_list_value_bytes = original_list_limit;
    else
      delete_property($server_options, "max_list_value_bytes");
    endif
  except (ANY)
    cleanup_failed = 1;
  endtry
  try
    if (had_catchable)
      $server_options.max_concat_catchable = original_catchable;
    else
      delete_property($server_options, "max_concat_catchable");
    endif
  except (ANY)
    cleanup_failed = 1;
  endtry
  try
    load_server_options();
  except (ANY)
    cleanup_failed = 1;
  endtry
endtry
if (cleanup_failed)
  raise(E_INVARG);
endif
return outcome;
`

func TestLimitsDynamicStatementRestoresListLimit(t *testing.T) {
	s := NewRuntime(serverOptionsVerbStore(t, limitsDynamicStatement))
	defer s.Stop()

	result := s.CallVerb(1, "go", nil, 2)
	if result.Flow == types.FlowException {
		t.Fatalf("go raised %s", result.Error)
	}
	if got := result.Val.String(); got != "E_QUOTA" {
		t.Fatalf("outcome = %s, want E_QUOTA", got)
	}
	if limit := s.Session().GetMaxListValueBytes(); limit != 64537861 {
		t.Fatalf("after the task: session max_list_value_bytes = %d, want the default 64537861", limit)
	}
}

// features/limits_dynamic.yaml lowers max_list_value_bytes, reloads, then in
// its finally deletes the property and reloads again, all in one committing
// task. The second reload must see the delete, so the published limit is the
// default again; language/parser_resource_limits.yaml runs next and builds a
// 5 KB list literal. Toast passes that pair.
func TestLoadServerOptionsSeesSameTaskPropertyDelete(t *testing.T) {
	const code = `add_property(#1, "max_list_value_bytes", 5000, {player, "r"});` +
		`load_server_options();` +
		`delete_property(#1, "max_list_value_bytes");` +
		`load_server_options();` +
		`return properties(#1);`
	s := NewRuntime(serverOptionsVerbStore(t, code))
	defer s.Stop()

	result := s.CallVerb(1, "go", nil, 2)
	if result.Flow == types.FlowException {
		t.Fatalf("go raised %s", result.Error)
	}
	if got := result.Val.String(); got != "{}" {
		t.Fatalf("properties(#1) after delete = %s, want {}", got)
	}
	if limit := s.Session().GetMaxListValueBytes(); limit != 64537861 {
		t.Fatalf("after the task: session max_list_value_bytes = %d, want the default 64537861", limit)
	}
}
