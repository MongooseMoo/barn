// The workflow owns scheduling; this validates its bounded dedicated selector.
const GROUP = 'barn-bug-discovery';

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
  const mode = process.env.COMPUTE_MODE;
  if (!['legacy', 'dedicated-once', 'dedicated-continuous'].includes(mode)) throw new Error('Invalid compute mode');
  let result;
  if (mode === 'legacy') {
    result = {runner: JSON.parse(process.env.CI_RUNNER_LABELS || '["ubuntu-latest"]'), parallel: 2};
  } else {
    if (context.ref !== 'refs/heads/master') throw new Error('Dedicated compute requires trusted master');
    result = dedicated(process.env);
    if (mode === 'dedicated-continuous' && process.env.FUZZ_CONTINUOUS_ENABLED !== 'true') {
      throw new Error('Continuous discovery is stopped');
    }
  }
  core.setOutput('runner', JSON.stringify(result.runner));
  core.setOutput('parallel', result.parallel);
  core.info(JSON.stringify({mode, ...result}));
}

async function replenish({github, context, core}) {
  const source = context.payload.workflow_run;
  if (context.eventName !== 'workflow_run' || context.ref !== 'refs/heads/master' ||
      source?.event !== 'workflow_dispatch' || source.head_branch !== 'master' ||
      source.path !== '.github/workflows/fuzz.yml' || source.conclusion !== 'success' ||
      source.display_title !== 'Nightly fuzz / dedicated-continuous' ||
      source.head_repository?.full_name !== `${context.repo.owner}/${context.repo.repo}`) {
    throw new Error('Replenishment requires a successful trusted continuous dispatch');
  }
  // This is a new completed-event run: its vars are evaluated independently
  // of the finished campaign. No variables API permission or extra token.
  if (process.env.FUZZ_CONTINUOUS_ENABLED !== 'true') {
    core.info('Continuous discovery is stopped; no successor submitted');
    return;
  }
  dedicated(process.env);
  const {data: latest} = await github.rest.actions.listWorkflowRuns({
    ...context.repo, workflow_id: 'fuzz.yml', event: 'workflow_dispatch', per_page: 1
  });
  if (latest.workflow_runs[0]?.id !== source.id) {
    core.info('A newer fuzz dispatch exists; no successor submitted');
    return;
  }
  for (const status of ['queued', 'in_progress', 'waiting', 'pending', 'requested']) {
    const runs = await github.paginate(github.rest.actions.listWorkflowRuns, {
      ...context.repo, workflow_id: 'fuzz.yml', event: 'workflow_dispatch', status, per_page: 100
    });
    if (runs.some(run => run.id !== context.runId)) {
      core.info('Another fuzz cycle already exists; no successor submitted');
      return;
    }
  }
  await github.rest.actions.createWorkflowDispatch({
    ...context.repo, workflow_id: 'fuzz.yml', ref: 'master',
    inputs: {'compute-mode': 'dedicated-continuous'}
  });
  core.info('Submitted one dedicated successor');
}

module.exports = {dedicated, configure, replenish};
