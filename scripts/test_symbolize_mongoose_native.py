"""Offline mapping/identity/denominator contracts; never attach to a process."""
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import symbolize_mongoose_native as symbols


def mmap(start="0x8000", length="0x4000", offset="0x2000", identity="<abc>", dso="/capture/moo"):
    return f"0/0 0.000000000: PERF_RECORD_MMAP2 7/7: [{start}({length}) @ {offset} {identity}]: r-xp {dso}\n"


def sample(ip="9002", period=5, dso="/capture/moo", tid=7):
    return f"7/{tid} 1.000000000: {period} cpu-clock:u: {ip} ({dso})\n"


class SymbolizerTests(unittest.TestCase):
    def translate(self, text, segments=None):
        mappings, samples = symbols.parse_samples(text)
        value, reason = symbols.address(samples[0], mappings, segments or [
            {"offset": 0x2000, "vaddr": 0x4000, "filesz": 0x3000, "executable": True}],
            "/capture/moo", "abc", 7)
        return value, reason

    def test_mapping_offset_and_elf_va_are_distinct_and_not_guessed_base(self):
        self.assertEqual(self.translate(mmap() + sample()), (0x5002, None))

    def test_future_mapping_cannot_resolve_earlier_sample(self):
        self.assertEqual(self.translate(sample() + mmap()), (None, "no_prior_matching_mapping"))

    def test_end_boundary_and_file_padding_remain_unresolved(self):
        self.assertEqual(self.translate(mmap() + sample("c000"))[1], "no_prior_matching_mapping")
        self.assertEqual(self.translate(mmap() + sample("b000"))[1], "file_offset_not_in_unique_executable_pt_load")

    def test_replaced_mapping_cannot_reuse_previous_build_identity(self):
        self.assertEqual(self.translate(mmap() + mmap(identity="<def>") + sample())[1],
                         "captured_build_id_missing_or_mismatched")

    def test_missing_or_wrong_build_id_never_resolves(self):
        for identity in ("00:00 0 0", "<def>"):
            with self.subTest(identity=identity):
                self.assertEqual(self.translate(mmap(identity=identity) + sample())[1],
                                 "captured_build_id_missing_or_mismatched")

    def test_other_thread_and_library_not_borrow_executable_identity(self):
        self.assertEqual(self.translate(mmap() + sample(tid=8))[1], "different_thread_or_process")
        self.assertEqual(self.translate(mmap() + sample(dso="/lib/libc.so.6"))[1],
                         "library_or_other_dso_not_sha_pinned")

    def test_ambiguous_elf_load_segments_rejected(self):
        segment = {"offset": 0x2000, "vaddr": 0x4000, "filesz": 0x3000, "executable": True}
        self.assertEqual(self.translate(mmap() + sample(), [segment, segment])[1],
                         "file_offset_not_in_unique_executable_pt_load")

    def test_malformed_unknown_and_lost_records_fail_instead_of_disappearing(self):
        for text in ("bad sample", "PERF_RECORD_LOST 10", mmap().replace("0x4000", "bad"), sample(period=0)):
            with self.subTest(text=text), self.assertRaises(ValueError):
                symbols.parse_samples(text)

    def test_addr2line_requires_address_order_and_preserves_unknown_lines(self):
        result = symbols.parse_addr2line("0x5002\nknown_function\nsource.cc:?\n", [0x5002])
        self.assertEqual(result[0x5002]["function"], "known_function")
        self.assertFalse(result[0x5002]["source_line_available"])
        with self.assertRaises(ValueError):
            symbols.parse_addr2line("0x5003\nknown_function\nsource.cc:10\n", [0x5002])

    def test_nm_accepts_only_bounded_code_symbols(self):
        result = symbols.nm_functions("00005000 00000010 t function\n00006000 00000000 T zero\n00007000 00000010 D data\n")
        self.assertEqual(result, [{"start": 0x5000, "size": 0x10, "function": "function"}])

    def test_addr2line_nearest_symbol_without_matching_bound_is_unresolved(self):
        symbol = {"function": "previous_function", "source_location": "source.cc:?",
                  "source_line_available": False}
        result = symbols.bounded_symbol(0x5002, symbol,
            [{"start": 0x4000, "size": 0x10, "function": "previous_function"}])
        self.assertIsNone(result["function"])
        self.assertEqual(result["unresolved_reason"], "no_matching_bounded_function_symbol")

    def test_nm_fallback_requires_one_unique_name_containing_address(self):
        symbol = {"function": None, "source_location": "??:0", "source_line_available": False}
        table = [{"start": 0x5000, "size": 0x10, "function": "bounded_function"}]
        result = symbols.bounded_symbol(0x5002, symbol, table)
        self.assertEqual(result["function"], "bounded_function")
        self.assertEqual(result["nm_symbol_bounds"], table)
        self.assertIsNone(symbols.bounded_symbol(0x5010, symbol, table)["function"])
        self.assertIsNone(symbols.bounded_symbol(0x5002, symbol, table +
            [{"start": 0x5000, "size": 0x10, "function": "conflicting_alias"}])["function"])

    def setup_capture(self, root, elf_sha=None, data_sha=None):
        native = root / "native"; native.mkdir()
        elf = root / "moo"; elf.write_bytes(b"pinned fixture executable")
        (native / "perf.data").write_bytes(b"pinned fixture capture")
        manifest = {"status": "collected", "callgraph": "none", "sample_count": 2,
                    "target": {"executable": "/capture/moo", "main_tid": 7, "sha256": elf_sha or symbols.sha(elf)},
                    "data_sha256": data_sha or symbols.sha(native / "perf.data")}
        tools = {}
        for name in ("perf", "readelf", "addr2line", "nm"):
            path = root / (name + "-tool"); path.write_bytes(name.encode()); tools[name] = str(path)
        manifest["capability"] = {"perf": tools["perf"], "perf_sha256": symbols.sha(tools["perf"])}
        (native / "native.json").write_text(json.dumps(manifest))
        return native, elf, tools

    def test_full_offline_pipeline_keeps_unresolved_weights_in_denominator(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary); native, elf, tools = self.setup_capture(root)
            def command(argv, output, name, commands, input_text=None):
                commands.append({"argv": argv, "exit_code": 0})
                return {"perf-mappings-samples": mmap() + sample() + sample("10000", 15, "/lib/libc.so.6"),
                        "elf-program-headers": "LOAD 0x2000 0x4000 0x4000 0x3000 0x3000 R E 0x1000\n",
                        "elf-notes": "Build ID: abc\n", "addr2line": "0x5002\nknown_function\nsource.cc:?\n",
                        "nm-functions": "00005000 00000010 t known_function\n"}.get(name, "version")
            with patch.object(symbols.shutil, "which", side_effect=lambda name: tools[name]), \
                    patch.object(symbols, "command", side_effect=command):
                report = symbols.run(native, root / "result", elf)
            self.assertEqual(report["status"], "complete", report)
            self.assertEqual(report["sample_count"], 2)
            self.assertEqual(report["sample_period_sum"], 20)
            self.assertEqual(report["functions"][0]["period_share_percent"], 25)
            self.assertEqual(report["unresolved"][0]["period_share_percent"], 75)
            rows = json.loads((root / "result/samples.json").read_text())
            self.assertEqual(sum(row["period"] for row in rows), 20)
            self.assertEqual(len(rows), 2)
            self.assertEqual(report["input_hashes"], report["final_input_hashes"])

    def test_changed_elf_or_data_hash_fails_before_tool_launch(self):
        for field in ("elf_sha", "data_sha"):
            with self.subTest(field=field), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary); native, elf, _ = self.setup_capture(root, **{field: "0" * 64})
                with patch.object(symbols, "command") as command:
                    result = symbols.run(native, root / "result", elf)
                self.assertEqual(result["status"], "failed")
                self.assertIn("SHA differs", result["reason"])
                command.assert_not_called()

    def test_preexisting_output_refused_and_preserved(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary); marker = root / "symbols.json"; marker.write_bytes(b"owned evidence")
            with self.assertRaises(FileExistsError):
                symbols.run(root, root)
            self.assertEqual(marker.read_bytes(), b"owned evidence")


if __name__ == "__main__":
    unittest.main()
