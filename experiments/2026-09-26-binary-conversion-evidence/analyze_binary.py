import json,math,random,re,statistics
from pathlib import Path
root=Path(__file__).parent
def parse(path):
 rows={}
 for line in path.read_text(encoding='utf-8-sig').splitlines():
  m=re.match(r'(Benchmark\S+)-\d+\s+\d+\s+([\d.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op',line)
  if m:rows[m[1]]=[float(m[2]),int(m[3]),int(m[4])]
 if len(rows)!=10:raise RuntimeError((path,rows))
 return rows
base=[parse(root/f'binary-base-{i}.txt') for i in range(10)]
cand=[parse(root/f'binary-candidate-{i}.txt') for i in range(10)]
rng=random.Random(260926)
def summary(xs):
 boots=sorted(statistics.median(rng.choices(xs,k=10)) for _ in range(10000))
 return {'median':statistics.median(xs),'range':[min(xs),max(xs)],'ci95':[boots[249],boots[9749]]}
out={}
for row in base[0]:
 out[row]={'ns':summary([cand[i][row][0]/base[i][row][0] for i in range(10)])}
 if base[0][row][1]:out[row]['bytes']=summary([cand[i][row][1]/base[i][row][1] for i in range(10)])
 else:
  assert all(cand[i][row][1:]==[0,0] for i in range(10))
  out[row]['bytes']='zero on both'
rows=[row for row in base[0] if '/binary' in row or '/escaped' in row]
assert len(rows)==4
out['primary_geomean_bytes']=summary([math.exp(sum(math.log(cand[i][row][1]/base[i][row][1]) for row in rows)/4) for i in range(10)])
print(json.dumps(out,indent=2))
