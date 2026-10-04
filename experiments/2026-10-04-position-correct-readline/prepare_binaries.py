"""Freeze timed and separately instrumented readline binaries in WSL."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--baseline", required=True)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=True)
    source = Path("builtins/fileio.go").resolve()
    tests = Path("builtins/fileio_readline_buffer_test.go").resolve()
    baseline = subprocess.run(["git", "show", f"{args.baseline}:builtins/fileio.go"],
                              check=True, capture_output=True, text=True).stdout
    counter = '''
var readlineReadCalls int
func countedReadlineRead(file *os.File, buf []byte) (int, error) {
    readlineReadCalls++
    return file.Read(buf)
}
func TestReadlineReadCallBudget(t *testing.T) {
    for _, size := range []int{0, 1, 32, 1024, 65536} {
        t.Run(fmt.Sprint(size), func(t *testing.T) {
            ctx, id := readlineFixture(t, strings.Repeat("x", size)+"\\n", "r-tf")
            readlineReadCalls = 0
            requireFileString(t, builtinFileReadline(ctx, []types.Value{id}), strings.Repeat("x", size))
            t.Logf("bytes=%d Go_Read_calls=%d", size+1, readlineReadCalls)
            budget := 2 + (size+4095)/4096
            if readlineReadCalls > budget { t.Fatalf("read calls %d > budget %d", readlineReadCalls, budget) }
        })
    }
}
'''
    for variant, content in (("baseline", baseline), ("candidate", source.read_text())):
        for measured in (False, True):
            label = variant + ("-count" if measured else "")
            frozen = output / f"{label}.go"
            if measured:
                if content.count("h.file.Read(tmp)") != 1:
                    raise ValueError("unexpected readline read anchor")
                frozen.write_text(content.replace("h.file.Read(tmp)", "countedReadlineRead(h.file, tmp)"))
            else:
                frozen.write_text(content)
            replacements = {str(source): str(frozen)}
            if measured:
                counted_tests = output / "count_test.go"
                counted_tests.write_text(tests.read_text()+counter)
                replacements[str(tests)] = str(counted_tests)
            overlay = output / f"{label}.json"
            overlay.write_text(json.dumps({"Replace": replacements}))
            binary = output / f"{label}.test"
            subprocess.run(["go", "test", "-c", f"-overlay={overlay}", "./builtins", "-o", str(binary)], check=True)
            print(label, hashlib.sha256(binary.read_bytes()).hexdigest(), flush=True)
    print("test-source", hashlib.sha256(tests.read_bytes()).hexdigest())


if __name__ == "__main__":
    main()
