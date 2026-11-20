"""Attribution and race tests for passive executing-stack observations."""
import errno
import struct
import unittest

from sample_live_moo import StackReader, summarize


LAYOUT = {
    "pointer_size": 8, "num_size": 8, "int_size": 4, "activation_size": 160,
    "type_obj": 1, "type_anon": 140, "var_value": 0, "var_type": 8,
    "offsets": {"prog": 0, "error_pc": 40, "bi_func_pc": 44, "player": 96,
                "progr": 104, "recv": 112, "vloc": 120, "verb": 136, "verbname": 144},
}
ADDRESSES = {"running": 0x1000, "top": 0x1004, "stack": 0x1008, "task": 0x1010}


def activation(definer, receiver, name_pointer):
    data = bytearray(160)
    struct.pack_into("<Q", data, 0, 0x9000)
    struct.pack_into("<I", data, 40, 321)
    struct.pack_into("<qqqqI", data, 96, 7, 8, receiver, definer, 1)
    struct.pack_into("<QQ", data, 136, name_pointer, name_pointer)
    return bytes(data)


class Memory:
    def __init__(self):
        self.globals = bytearray(20)
        struct.pack_into("<IIQi", self.globals, 0, 1, 1, 0x5000, 42)
        self.stack = activation(100, 200, 0x7000) + activation(101, 201, 0x8000)
        self.reads = []

    def read(self, address, size):
        self.reads.append((address, size))
        for start, data in ((0x1000, self.globals), (0x5000, self.stack),
                            (0x7000, b"driver\0" + bytes(256)),
                            (0x8000, b"worker\0" + bytes(256))):
            if start <= address < start + len(data):
                return bytes(data[address - start:address - start + size])
        raise OSError(errno.EIO, "unmapped fixture address")

    def observe(self, read=None, max_frames=16):
        return StackReader(read or self.read, LAYOUT, ADDRESSES, max_frames).observe()


class ObservationTests(unittest.TestCase):
    def test_top_is_nested_callee_with_definition_distinct_from_receiver(self):
        row = Memory().observe()
        self.assertEqual(row["state"], "active_observation")
        self.assertEqual(row["task_id"], 42)
        self.assertEqual([(f["definer"], f["called_verb"], f["receiver"]) for f in row["frames"]],
                         [("#101", "worker", "#201"), ("#100", "driver", "#200")])
        self.assertIsNone(row["frames"][0]["source_line"])
        self.assertNotIn("program", row["frames"][0])
        self.assertNotIn("verb_pointer", row["frames"][0])

    def test_outside_interpreter_does_not_read_stale_frames(self):
        memory = Memory()
        struct.pack_into("<I", memory.globals, 0, 0)
        self.assertEqual(memory.observe(), {"state": "outside_interpreter"})
        self.assertEqual(memory.reads, [(0x1000, 4), (0x1000, 4)])

    def test_changed_task_is_rejected_without_attribution(self):
        memory = Memory()
        task_reads = 0
        def read(address, size):
            nonlocal task_reads
            if address == ADDRESSES["task"]:
                task_reads += 1
                if task_reads == 2:
                    struct.pack_into("<i", memory.globals, 16, 99)
            return memory.read(address, size)
        row = memory.observe(read)
        self.assertEqual(row["state"], "unstable")
        self.assertNotIn("frames", row)

    def test_changed_activation_is_rejected(self):
        memory = Memory()
        stack_reads = 0
        def read(address, size):
            nonlocal stack_reads
            if address == 0x5000:
                stack_reads += 1
                if stack_reads == 2:
                    memory.stack = activation(100, 200, 0x7000) + activation(202, 201, 0x8000)
            return memory.read(address, size)
        self.assertEqual(memory.observe(read)["state"], "unstable")

    def test_changed_string_storage_is_rejected(self):
        memory = Memory()
        name_reads = 0
        def read(address, size):
            nonlocal name_reads
            if address == 0x8000:
                name_reads += 1
                if name_reads >= 3:
                    return b"reused\0" + bytes(size - 7)
            return memory.read(address, size)
        self.assertEqual(memory.observe(read)["state"], "unstable")

    def test_inaccessible_memory_preserves_native_errno(self):
        memory = Memory()
        def read(address, size):
            if address == 0x5000:
                raise PermissionError(errno.EPERM, "fixture denied")
            return memory.read(address, size)
        row = memory.observe(read)
        self.assertEqual(row["state"], "unreadable")
        self.assertEqual(row["errno"], errno.EPERM)
        self.assertNotIn("frames", row)

    def test_short_frame_read_is_unknown(self):
        memory = Memory()
        memory.stack = memory.stack[:-1]
        self.assertEqual(memory.observe()["state"], "unreadable")

    def test_underflowed_top_index_is_rejected_before_memory_read(self):
        memory = Memory()
        struct.pack_into("<I", memory.globals, 4, 0xffffffff)
        self.assertEqual(memory.observe()["state"], "unstable")
        self.assertFalse(any(address >= 0x5000 for address, _ in memory.reads))

    def test_bounded_stack_retains_callee_and_reports_omitted_callers(self):
        row = Memory().observe(max_frames=1)
        self.assertEqual(row["frames"][0]["called_verb"], "worker")
        self.assertEqual(row["stack_depth"], 2)
        self.assertEqual(row["frames_omitted"], 1)

    def test_no_profile_is_inferred_from_unknown_observations(self):
        result = summarize([{"state": "unstable"}, {"state": "outside_interpreter"}])
        self.assertEqual(result["top_observed_verbs"], [])
        self.assertIsNone(result["median_active_read_s"])
        self.assertEqual(result["sample_states"], {"unstable": 1, "outside_interpreter": 1})


if __name__ == "__main__":
    unittest.main()
