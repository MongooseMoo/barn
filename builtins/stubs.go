package builtins

import "barn/types"

// stubNotImplemented is a placeholder for builtins that exist in ToastStunt
// but are not yet implemented in Barn. Returns E_ARGS to signal that the
// function exists but was called incorrectly (since we don't validate args yet).
func stubNotImplemented(ctx *types.TaskContext, args []types.Value) types.Result {
	return types.Err(types.E_ARGS)
}

// RegisterStubBuiltins registers placeholder stubs for all ToastStunt builtins
// that Barn recognizes but has not yet implemented. These ensure that
// function_info() and presence checks work, while actual calls get E_ARGS.
func (r *Registry) RegisterStubBuiltins() {
	// Only register if not already registered (avoid overwriting real implementations)
	stub := func(name string) {
		if !r.Has(name) {
			r.Register(name, stubNotImplemented)
		}
	}

	// === Core builtins (builtin_presence_core) ===

	// Math extensions
	stub("acosh")
	stub("asinh")
	stub("atan2")
	stub("atanh")
	stub("cbrt")
	stub("distance")
	stub("frandom")
	stub("relative_heading")
	stub("reseed_random")
	stub("round")
	stub("simplex_noise")

	// String extensions
	stub("chr")
	stub("parse_ansi")
	stub("remove_ansi")

	// List extensions
	stub("all_members")

	// Introspection / system
	stub("background_test")
	stub("buffered_output_length")
	stub("call_function")
	stub("connection_options")
	stub("db_disk_size")
	stub("dump_database")
	stub("finished_tasks")
	stub("flush_input")
	stub("force_input")
	stub("function_info")
	stub("listen")
	stub("locate_by_name")
	stub("locations")
	stub("log_cache_stats")
	stub("malloc_stats")
	stub("memory_usage")
	stub("next_recycled_object")
	stub("open_network_connection")
	stub("output_delimiters")
	stub("owned_objects")
	stub("queue_info")
	stub("read")
	stub("recreate")
	stub("recycled_objects")
	stub("reset_max_object")
	stub("set_thread_mode")
	stub("shutdown")
	stub("task_perms")
	stub("thread_pool")
	stub("threads")
	stub("unlisten")
	stub("usage")
	stub("verb_cache_stats")
	stub("waif_stats")

	// === Extension builtins (builtin_presence_extensions) ===

	// Crypto
	stub("argon2")
	stub("argon2_verify")

	// Network
	stub("curl")

	// File I/O
	stub("file_chmod")
	stub("file_close")
	stub("file_count_lines")
	stub("file_eof")
	stub("file_flush")
	stub("file_grep")
	stub("file_handles")
	stub("file_last_access")
	stub("file_last_change")
	stub("file_last_modify")
	stub("file_list")
	stub("file_mkdir")
	stub("file_mode")
	stub("file_name")
	stub("file_open")
	stub("file_openmode")
	stub("file_read")
	stub("file_readline")
	stub("file_readlines")
	stub("file_remove")
	stub("file_rename")
	stub("file_rmdir")
	stub("file_seek")
	stub("file_size")
	stub("file_stat")
	stub("file_tell")
	stub("file_type")
	stub("file_write")
	stub("file_writeline")

	// PCRE
	stub("pcre_cache_stats")
	stub("pcre_match")
	stub("pcre_replace")

	// Misc extensions
	stub("read_stdin")
	stub("spellcheck")

	// SQLite
	stub("sqlite_close")
	stub("sqlite_execute")
	stub("sqlite_handles")
	stub("sqlite_info")
	stub("sqlite_interrupt")
	stub("sqlite_last_insert_row_id")
	stub("sqlite_limit")
	stub("sqlite_open")
	stub("sqlite_query")

	// URL encoding
	stub("url_decode")
	stub("url_encode")
}
