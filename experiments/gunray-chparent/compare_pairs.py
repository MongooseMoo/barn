"""Check each pair of server histories against its precomputed relations."""

import csv
import json
import sys
from pathlib import Path
from xml.etree import ElementTree


def compare(evidence, engine):
    prediction = json.loads((evidence / "predictions.json").read_text())
    directory = evidence / f"model-{engine}"
    if (directory / "exit-code").read_text().strip() != "0":
        raise ValueError(f"{engine} did not finish cleanly; inspect results.xml and session.log")
    tests = ElementTree.parse(directory / "results.xml").findall(".//testcase")
    names = [node.attrib["name"] for node in tests]
    arms = [f"gunray_pairs_{pair}_{arm}" for pair in prediction["pairs"] for arm in ("left", "right")]
    assert len(names) == 1 + len(arms) and names.count("test_capability_admission") == 1
    assert all(sum(name.endswith("::" + arm + "]") for name in names) == 1 for arm in arms)
    assert all(node.find(tag) is None for node in tests for tag in ("failure", "error", "skipped"))
    observations = {}
    for line in (directory / "observations.jsonl").read_text().splitlines():
        record = json.loads(line)
        assert record["result"]["success"] and record["case"] not in observations
        observations[record["case"]] = record["result"]["value"]

    violations, absolute = [], []
    with (directory / "pairs.csv").open("w", newline="") as out:
        writer = csv.writer(out)
        writer.writerow(["pair", "phase", "child", "expected_left", "expected_right",
                         "observed_left", "observed_right", "relation_ok", "predictions_ok"])
        for pair, predictions in prediction["pairs"].items():
            for phase in predictions["left"]["expected"]:
                actual = [observations.pop(f"gunray_pairs_{pair}_{arm}_{phase}") for arm in ("left", "right")]
                expected = [predictions[arm]["expected"][phase] for arm in ("left", "right")]
                for child, left, right, x, y in zip(prediction["children"], *expected, *actual, strict=True):
                    # The control may differ in clear bit alone. Check each component's
                    # equality relation, so an unrelated difference cannot satisfy it.
                    relation_ok = (len(x) == len(left) and len(y) == len(right) and
                                   [a == b for a, b in zip(x, y)] == [a == b for a, b in zip(left, right)])
                    predictions_ok = (left, right) == (x, y)
                    row = [pair, phase, child, left, right, x, y, relation_ok, predictions_ok]
                    writer.writerow(row)
                    if not relation_ok:
                        violations.append(row)
                    if not predictions_ok:
                        absolute.append(row)
    assert not observations, f"Unexpected observations: {observations.keys()}"
    print(f"{engine}: {len(violations)} pair violations, {len(absolute)} rows differing from predicted values")
    for bad in (violations, absolute):
        if bad:
            print("First discrepancy:", bad[0])
    return not (violations or absolute)


if __name__ == "__main__":
    sys.exit(0 if compare(Path(sys.argv[1]), sys.argv[2]) else 1)
