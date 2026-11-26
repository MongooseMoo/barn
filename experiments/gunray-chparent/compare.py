"""Join complete managed observations by case and object, retaining every value."""

import csv
import json
import sys
from pathlib import Path
from xml.etree import ElementTree


def collect(evidence, engines):
    cases = json.loads((evidence / "predictions.json").read_text())["cases"]
    rows = {
        (name, label): {"case": name, "object": label, "gunray": pair}
        for name, case in cases.items()
        for label, pair in zip(case["labels"], case["expected"], strict=True)
    }
    for engine in engines:
        directory = evidence / f"model-{engine}"
        tests = ElementTree.parse(directory / "results.xml").findall(".//testcase")
        [admission] = [item for item in tests if item.get("name") == "test_capability_admission"]
        if any(admission.find(tag) is not None for tag in ("error", "failure", "skipped")):
            raise ValueError(f"{engine}: admission did not pass")
        observations = [json.loads(line) for line in (directory / "observations.jsonl").read_text().splitlines()]
        if len(observations) != len(cases) or {r["case"] for r in observations} != cases.keys():
            raise ValueError(f"{engine}: incomplete or duplicate observations")
        for observation in observations:
            name = observation["case"]
            result = observation["result"]
            if not result["success"]:
                raise ValueError(f"{engine}/{name}: {result}")
            for label, pair in zip(cases[name]["labels"], result["value"], strict=True):
                rows[name, label][engine] = pair
            [test] = [t for t in tests if t.get("name", "").endswith("::" + name + "]")]
            mismatch = result["value"] != cases[name]["expected"]
            if test.find("skipped") is not None or test.find("error") is not None or (test.find("failure") is not None) != mismatch:
                raise ValueError(f"{engine}/{name}: observation and test outcome disagree")
        differences = [r for r in rows.values() if r["gunray"] != r[engine]]
        print(f"{engine}: {len(differences)} differing slots across {len({r['case'] for r in differences})} of {len(cases)} cases")
    with (evidence / "observations.csv").open("w", newline="") as out:
        writer = csv.DictWriter(out, fieldnames=["case", "object", "gunray", *engines])
        writer.writeheader()
        writer.writerows(rows.values())


if __name__ == "__main__":
    collect(Path(sys.argv[1]), sys.argv[2:] or ("toast", "before", "after", "current"))
