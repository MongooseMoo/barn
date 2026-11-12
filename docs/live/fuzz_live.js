// What the live fuzz page says, from GitHub API records. No DOM here, so
// scripts/test_live_fuzz_page.py runs it in Node.
"use strict";
(function (live) {
  // Jobs that are not native fuzz lanes (fuzz_compute.cjs NOT_LANES).
  const ROUTE = "Validate compute routing", DIFFERENTIAL = "ToastDifferential";
  const ACCEPT = "Accept discovery cycle", SUCCESSOR = "Submit continuous successor";
  const NOT_LANES = new Set([ROUTE, DIFFERENTIAL, ACCEPT, SUCCESSOR]);
  // The one step allowed to fail when a job finds something (fuzz_compute.cjs accept).
  const campaignStep = name => name === DIFFERENTIAL
    ? "Compare retained public source on Barn and Toast" : `Fuzz ${name}`;
  const WAITING = new Set(["queued", "waiting", "pending", "requested"]);
  // Issue reports finish in the job's last steps; allow for clock skew.
  const SLACK_MS = 60 * 1000;
  const CLOSE_MS = 5 * 1000;
  const DAY_MS = 24 * 60 * 60 * 1000;
  const ms = iso => Date.parse(iso);

  // fuzz_report.py publish(): "Nightly fuzz failure: <target>" for a finding,
  // "Nightly fuzz infrastructure failure: <target>" for a machine problem.
  function report(issue) {
    const title = /^Nightly fuzz (infrastructure )?failure: (\S+)$/.exec(issue.title || "");
    if (!title || issue.pull_request) return null;
    const run = /\/actions\/runs\/(\d+)\//.exec(issue.body || "");
    return {machine: Boolean(title[1]), target: title[2], run: run ? Number(run[1]) : null};
  }

  // A new report names this run in its body; a repeat is a comment on an
  // existing report, seen only as that issue's last update inside the job.
  // An issue whose last update is its own closing (a repair PR merging) was
  // not commented on.
  function updatedDuring(issue, start, end) {
    const updated = ms(issue.updated_at);
    const closing = issue.closed_at && Math.abs(updated - ms(issue.closed_at)) <= CLOSE_MS;
    return !closing && updated >= start && updated <= end;
  }

  function reports(job, runId, issues, now) {
    const start = ms(job.started_at), end = job.completed_at ? ms(job.completed_at) + SLACK_MS : ms(now);
    const out = [];
    for (const issue of issues) {
      const r = report(issue);
      if (!r || r.target !== job.name) continue;
      const fresh = r.run === runId;
      if (!fresh && !updatedDuring(issue, start, end)) continue;
      out.push(r.machine
        ? {kind: "machine", issue, why: fresh ? "machine problem, new report" : "machine problem, already reported in"}
        : {kind: fresh ? "bug" : "repeat", issue});
    }
    // Bugs first, then repeats, then machine problems.
    const order = {bug: 0, repeat: 1, machine: 2};
    return out.sort((a, b) => order[a.kind] - order[b.kind]);
  }

  function classifyJob(job, runId, issues, now) {
    if (WAITING.has(job.status)) return [{kind: "waiting"}];
    if (job.status === "in_progress") return [{kind: "running"}, ...reports(job, runId, issues, now)];
    if (job.conclusion === "success") return [{kind: "clean"}];
    if (job.conclusion === "skipped") return [{kind: "skipped"}];
    if (job.conclusion !== "failure") {
      return [{kind: "machine", why: `job ${String(job.conclusion).replace("_", " ")}`}];
    }
    const failed = (job.steps || []).filter(s => s.conclusion === "failure").map(s => s.name);
    if (!failed.length) {
      return [{kind: "machine", why: "job failed outside its steps (runner lost or stopped)"}];
    }
    const other = failed.filter(name => name !== campaignStep(job.name));
    const found = failed.length > other.length ? reports(job, runId, issues, now) : [];
    const out = other.length ? [{kind: "machine", why: `step failed: ${other.join(", ")}`}] : [];
    out.push(...found);
    return out.length ? out : [{kind: "unmatched"}];
  }

  const lanes = jobs => jobs.filter(j => !NOT_LANES.has(j.name));

  function progress(jobs) {
    const l = lanes(jobs);
    return {total: l.length, done: l.filter(j => j.status === "completed").length,
      running: l.filter(j => j.status === "in_progress").length,
      waiting: l.filter(j => WAITING.has(j.status)).length};
  }

  const progressText = p => `${p.done} of ${p.total} lanes done` +
    (p.running ? `, ${p.running} running` : "") + (p.waiting ? `, ${p.waiting} waiting` : "");

  // fuzz_compute.cjs accept(): the cycle is healthy when every lane ran its
  // time and kept its evidence, whatever the lanes found.
  function health(jobs) {
    const accept = jobs.find(j => j.name === ACCEPT);
    if (!accept || accept.status !== "completed") return "pending";
    return {success: "healthy", failure: "not healthy"}[accept.conclusion] || "not checked";
  }

  // claude-fix.yml pushes each automatic repair on claude/issue-<N>.
  function repairFor(number, pulls) {
    return pulls.find(p => p.head && p.head.ref === `claude/issue-${number}`) || null;
  }

  function lastDay(issues, pulls, now) {
    const since = ms(now) - DAY_MS;
    const bugs = issues.filter(i => { const r = report(i); return r && !r.machine && ms(i.created_at) >= since; })
      .sort((a, b) => ms(b.created_at) - ms(a.created_at))
      .map(i => ({...i, pr: repairFor(i.number, pulls)}));
    const fixes = pulls.filter(p => p.head && p.head.ref.startsWith("claude/issue-") && p.merged_at &&
      ms(p.merged_at) >= since).sort((a, b) => ms(b.merged_at) - ms(a.merged_at));
    return {bugs, fixes};
  }

  const count = (n, one, many) => `${n} ${n === 1 ? one : many}`;

  // The page's one announced line. It holds no relative time, so it changes
  // (and a screen reader speaks it) only when the machine's state does.
  function summary({current, currentJobs, previous, issues, pulls, now, locale}) {
    const day = lastDay(issues, pulls, now);
    const tail = `Last 24 hours: ${count(day.bugs.length, "new bug", "new bugs")} found, ` +
      `${count(day.fixes.length, "fix", "fixes")} merged.`;
    let head;
    if (!current) {
      head = previous
        ? `Stopped: no cycle is running or queued; the last one ended at ${local(previous.updated_at, now, locale)}.`
        : "Stopped: no dedicated fuzz cycle found.";
    } else {
      const p = progress(currentJobs);
      const active = currentJobs.some(j => j.status === "in_progress");
      if (!p.total) head = "Starting a cycle.";
      else if (p.done === p.total) head = `Running now: all ${p.total} lanes done, finishing the cycle.`;
      else head = `${active ? "Running now" : "Waiting for a runner"}: ${progressText(p)}.`;
    }
    return `${head} ${tail}`;
  }

  function ago(iso, now) {
    const minutes = Math.floor((ms(now) - ms(iso)) / 60000);
    if (minutes < 1) return "just now";
    if (minutes >= 48 * 60) return `${Math.floor(minutes / 1440)} days ago`;
    return `${duration(minutes * 60000)} ago`;
  }

  function duration(milliseconds) {
    const minutes = Math.floor(milliseconds / 60000), h = Math.floor(minutes / 60), m = minutes % 60;
    if (!minutes) return "less than a minute";
    if (!h) return count(m, "minute", "minutes");
    return count(h, "hour", "hours") + (m ? ` ${count(m, "minute", "minutes")}` : "");
  }

  // The viewer's own time zone, named; the date only when it is not today.
  function local(iso, now, locale) {
    const date = new Date(iso);
    const today = new Date(now).toDateString() === date.toDateString();
    return date.toLocaleString(locale, today
      ? {hour: "numeric", minute: "2-digit", timeZoneName: "short"}
      : {weekday: "short", month: "short", day: "numeric", hour: "numeric", minute: "2-digit", timeZoneName: "short"});
  }

  const when = (iso, now, locale) => `${ago(iso, now)} (${local(iso, now, locale)})`;
  // How long a finished run took, from GitHub's run timestamps.
  const took = run => duration(ms(run.updated_at) - ms(run.created_at));

  Object.assign(live, {ACCEPT, DIFFERENTIAL, WAITING, lanes, classifyJob, progress, progressText,
    health, repairFor, lastDay, summary, ago, duration, local, when, took});
})(typeof module === "object" ? module.exports : (globalThis.FuzzLive = {}));
