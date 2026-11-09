// The workflow owns scheduling; this validates its bounded dedicated selector.
const GROUP = 'barn-bug-discovery';

function campaign() {
  const selected = process.env.FUZZ_CAMPAIGN || 'native-and-toast';
  if (!['native-and-toast', 'native-only'].includes(selected)) throw new Error('Invalid fuzz campaign');
  return selected;
}

function dedicated(config) {
  if (config.FUZZ_RUNNER_POLICY_VERIFIED !== 'true') throw new Error('Dedicated runner policy is not verified');
  const labels = JSON.parse(config.FUZZ_RUNNER_LABELS || 'null');
  if (!Array.isArray(labels) || !labels.length || labels.some(x => typeof x !== 'string' || !x.trim()) ||
      !labels.includes('barn-fuzz') || !labels.includes('self-hosted')) {
    throw new Error('FUZZ_RUNNER_LABELS must explicitly select self-hosted barn-fuzz runners');
  }
  const parallel = Number(config.FUZZ_MAX_PARALLEL || '2');
  if (![2, 4].includes(parallel)) throw new Error('FUZZ_MAX_PARALLEL must be 2 or 4');
  return {runner: {group: GROUP, labels}, parallel};
}

function configure({context, core}) {
  const selected = campaign();
  const mode = process.env.COMPUTE_MODE;
  if (!['legacy', 'dedicated-once', 'dedicated-continuous'].includes(mode)) throw new Error('Invalid compute mode');
  let result;
  if (mode === 'legacy') {
    // GitHub allows at most 256 matrix jobs. Let available runners set capacity.
    result = {runner: JSON.parse(process.env.CI_RUNNER_LABELS || '["ubuntu-latest"]'), parallel: 256};
  } else {
    if (context.ref !== 'refs/heads/master') throw new Error('Dedicated compute requires trusted master');
    result = dedicated(process.env);
    if (mode === 'dedicated-continuous' && process.env.FUZZ_CONTINUOUS_ENABLED !== 'true') {
      throw new Error('Continuous discovery is stopped');
    }
  }
  core.setOutput('runner', JSON.stringify(result.runner));
  core.setOutput('parallel', result.parallel);
  core.setOutput('campaign', selected);
  core.info(JSON.stringify({mode, campaign: selected, ...result}));
}

const ACCEPT = 'Accept discovery cycle';
const DIFFERENTIAL = 'ToastDifferential';
const NOT_LANES = new Set(['Validate compute routing', DIFFERENTIAL, ACCEPT]);

// A cycle is accepted when every lane completed measured work and kept its
// evidence. A saved finding or a real divergence leaves its job red and is
// still accepted; setup, zero-work, inconclusive or missing evidence is not.
async function accept({github, context, core}) {
  const selected = campaign();
  const fs = require('node:fs');
  const path = require('node:path');
  const jobs = await github.paginate(github.rest.actions.listJobsForWorkflowRun, {
    ...context.repo, run_id: context.runId, filter: 'latest', per_page: 100
  });
  const census = new Map();
  const walk = directory => {
    for (const entry of fs.readdirSync(directory, {withFileTypes: true})) {
      const file = path.join(directory, entry.name);
      if (entry.isDirectory()) walk(file);
      else if (entry.name === 'census.json') {
        const lane = JSON.parse(fs.readFileSync(file, 'utf8'));
        census.set(lane.target, lane);
      }
    }
  };
  if (fs.existsSync('.fuzz/retained')) walk('.fuzz/retained');
  const problems = [];
  // Only the step that found something may fail; a failed upload, cache save
  // or issue report is an evidence failure.
  const onlyFailed = (job, step) => {
    const failed = (job.steps || []).filter(s => s.conclusion === 'failure').map(s => s.name);
    return job.conclusion === 'success' ? failed.length === 0 : failed.length === 1 && failed[0] === step;
  };
  const lanes = jobs.filter(job => !NOT_LANES.has(job.name));
  if (!lanes.length) problems.push('no native lanes ran');
  for (const job of lanes) {
    const lane = census.get(job.name);
    if (!['success', 'failure'].includes(job.conclusion)) problems.push(`${job.name}: job ${job.conclusion}`);
    else if (!onlyFailed(job, `Fuzz ${job.name}`)) problems.push(`${job.name}: a step other than the campaign failed`);
    else if (!lane) problems.push(`${job.name}: no census`);
    else if (!(lane.completed_executions > 0)) problems.push(`${job.name}: zero measured executions`);
    else if (lane.campaign !== (job.conclusion === 'success' ? 'clean' : 'finding')) {
      problems.push(`${job.name}: campaign ${lane.campaign} with job ${job.conclusion}`);
    }
  }
  const differential = jobs.find(job => job.name === DIFFERENTIAL);
  const resultsFile = 'fuzz-differential/results.json';
  const outcome = fs.existsSync(resultsFile) ? JSON.parse(fs.readFileSync(resultsFile, 'utf8')).summary?.outcome : undefined;
  if (selected === 'native-only') {
    if (differential?.conclusion !== 'skipped' || fs.existsSync(resultsFile)) {
      problems.push(`${DIFFERENTIAL}: native-only requires a skipped comparison with no results`);
    }
  } else if (!differential || !['success', 'failure'].includes(differential.conclusion)) {
    problems.push(`${DIFFERENTIAL}: job ${differential?.conclusion ?? 'missing'}`);
  } else if (!onlyFailed(differential, 'Compare retained public source on Barn and Toast')) {
    problems.push(`${DIFFERENTIAL}: a step other than the comparison failed`);
  } else if (outcome !== (differential.conclusion === 'success' ? 'success' : 'divergence')) {
    problems.push(`${DIFFERENTIAL}: outcome ${outcome ?? 'missing'} with job ${differential.conclusion}`);
  }
  core.info(JSON.stringify({campaign: selected, lanes: lanes.length,
    differential: selected === 'native-only' ? 'not-run-native-only' : outcome ?? null, problems}));
  if (problems.length) throw new Error(`Cycle not accepted: ${problems.join('; ')}`);
}

async function replenish({github, context, core}) {
  const source = context.payload.workflow_run;
  const campaigns = new Map([
    ['Nightly fuzz / dedicated-continuous', 'native-and-toast'],
    ['Nightly fuzz / dedicated-continuous / native-only', 'native-only']
  ]);
  const selected = campaigns.get(source?.display_title);
  if (context.eventName !== 'workflow_run' || context.ref !== 'refs/heads/master' ||
      source?.event !== 'workflow_dispatch' || source.head_branch !== 'master' ||
      source.path !== '.github/workflows/fuzz.yml' || !['success', 'failure'].includes(source.conclusion) ||
      !selected ||
      source.head_repository?.full_name !== `${context.repo.owner}/${context.repo.repo}`) {
    throw new Error('Replenishment requires a completed trusted continuous dispatch');
  }
  // This is a new completed-event run: its vars are evaluated independently
  // of the finished campaign. No variables API permission or extra token.
  if (process.env.FUZZ_CONTINUOUS_ENABLED !== 'true') {
    core.info('Continuous discovery is stopped; no successor submitted');
    return;
  }
  dedicated(process.env);
  const sourceJobs = await github.paginate(github.rest.actions.listJobsForWorkflowRun, {
    ...context.repo, run_id: source.id, filter: 'latest', per_page: 100
  });
  if (!sourceJobs.some(job => job.name === ACCEPT && job.conclusion === 'success')) {
    throw new Error('The source cycle was not accepted; continuous discovery stops here');
  }
  // Only dedicated cycles compete with a successor: legacy runs (the nightly
  // workstation dispatch) use their own concurrency group and runner pool.
  const dedicatedRun = run => run.display_title?.startsWith('Nightly fuzz / dedicated-');
  const {data: latest} = await github.rest.actions.listWorkflowRuns({
    ...context.repo, workflow_id: 'fuzz.yml', event: 'workflow_dispatch', per_page: 100
  });
  if (latest.workflow_runs.find(dedicatedRun)?.id !== source.id) {
    core.info('A newer dedicated fuzz dispatch exists; no successor submitted');
    return;
  }
  for (const status of ['queued', 'in_progress', 'waiting', 'pending', 'requested']) {
    const runs = await github.paginate(github.rest.actions.listWorkflowRuns, {
      ...context.repo, workflow_id: 'fuzz.yml', event: 'workflow_dispatch', status, per_page: 100
    });
    if (runs.some(run => run.id !== context.runId && dedicatedRun(run))) {
      core.info('Another dedicated fuzz cycle already exists; no successor submitted');
      return;
    }
  }
  await github.rest.actions.createWorkflowDispatch({
    ...context.repo, workflow_id: 'fuzz.yml', ref: 'master',
    inputs: {'compute-mode': 'dedicated-continuous', campaign: selected}
  });
  core.info('Submitted one dedicated successor');
}

module.exports = {dedicated, configure, accept, replenish};
