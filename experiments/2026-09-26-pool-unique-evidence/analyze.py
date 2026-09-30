import json, math, random, re, statistics, sys
from pathlib import Path

root=Path(__file__).parent
name=sys.argv[1]
def parse(path):
 rows={}
 for line in path.read_text(encoding='utf-8-sig').splitlines():
  m=re.match(r'(Benchmark\S+)-\d+\s+\d+\s+([\d.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op',line)
  if m: rows[m[1]]=[float(m[2]),float(m[3]),float(m[4])]
 if len(rows)!=4: raise RuntimeError((path,rows))
 return rows
base=[parse(root/f'{name}-base-{i}.txt') for i in range(10)]
cand=[parse(root/f'{name}-candidate-{i}.txt') for i in range(10)]
rng=random.Random(260926)
def summary(xs):
 boots=sorted(statistics.median(rng.choices(xs,k=len(xs))) for _ in range(10000))
 return {'median':statistics.median(xs),'range':[min(xs),max(xs)],'ci95':[boots[249],boots[9749]]}
out={}
for row in base[0]:
 out[row]={metric:summary([cand[i][row][j]/base[i][row][j] for i in range(10)]) for j,metric in enumerate(['ns','bytes','allocs'])}
aggregate=[math.exp(sum(math.log(cand[i][row][1]/base[i][row][1]) for row in base[i])/4) for i in range(10)]
out['primary_geomean_bytes']=summary(aggregate)
print(json.dumps(out,indent=2))
