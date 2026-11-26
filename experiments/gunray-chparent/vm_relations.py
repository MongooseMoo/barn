"""Compare paired public observations directly, independently of expected values."""

import csv
import json
import sys
from pathlib import Path


def relations(data):
    for style in ("exact", "alias"):
        cold, warm = [data[f"dispatch_{style}_{arm}"] for arm in ("cold", "warm")]
        yield f"{style}: cold and warm dispatch agree", cold == warm
        yield f"{style}: rename and restore choose different ancestors", cold[0][3][0] != cold[1][3][0]
    for path in ("live", "ordinary"):
        noop, reject = [data[f"move_{path}_{arm}"] for arm in ("noop", "reject")]
        yield f"{path}: entire ordered graph agrees after every operation", [v[1] for v in noop] == [v[1] for v in reject]
        yield f"{path}: recursive attempt fails and retry succeeds", reject[4][0] == ["error", "E_RECMOVE"] and reject[6][0] == ["ok", 0]
    for arm in ("noop", "reject"):
        yield f"{arm}: move path agrees", data[f"move_live_{arm}"] == data[f"move_ordinary_{arm}"]
    for depth in (1, 2):
        for target in ("i", "j"):
            prefix = f"cleanup_{depth}_{target}_"
            baseline = data[prefix + "none"]
            for site in [*(f"clean{k}" for k in range(depth)), "outer_clean"]:
                yield f"{prefix}{site}: suspension preserves trace and terminal result", baseline == data[prefix + site]
        outer, inner = [data[f"cleanup_{depth}_{target}_none"][0] for target in ("i", "j")]
        yield f"depth {depth}: target changes trace and terminal iteration", outer[0] != inner[0] and outer[1] != inner[1]


def main(evidence):
    with (evidence / "relations.csv").open("w", newline="") as stream:
        writer = csv.writer(stream)
        writer.writerow(["engine", "relation", "holds"])
        failed = []
        for engine in ("toast", "current"):
            records = (evidence / f"model-{engine}" / "observations.jsonl").read_text().splitlines()
            data = {r["case"].removeprefix("gunray_vm_"): r["result"]["value"] for r in map(json.loads, records)}
            checks = list(relations(data))
            for relation, holds in checks:
                writer.writerow([engine, relation, holds])
                if not holds:
                    failed.append((engine, relation))
            print(f"{engine}: {sum(ok for _, ok in checks)}/{len(checks)} relational checks")
    if failed:
        raise SystemExit(f"Violated relations: {failed}")


if __name__ == "__main__":
    main(Path(sys.argv[1]))
