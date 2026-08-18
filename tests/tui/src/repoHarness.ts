/**
 * Repo fixtures for the TUI harness.
 *
 * The shapes live in tests/shared so the web E2E harness builds the same
 * workspaces; this module only re-exports them under the names the TUI specs
 * already use.
 */
export {
  createTempHelmdexRepo,
  rmTempRepo,
  repoPath,
  instancePath,
  fixturePath,
  isolatedStateEnv,
  type TempRepo,
  type RepoFixtureOptions
} from '../../shared/repoFixture';
