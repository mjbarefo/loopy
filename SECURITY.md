# Security

## Reporting

Report vulnerabilities privately via GitHub Security Advisories
(https://github.com/mjbarefo/loopy/security/advisories/new). Expect an
acknowledgement within a week. Please do not open public issues for
suspected vulnerabilities.

## The threat model, honestly

loopy is a local developer tool that runs commands you configure:

- **Registered agent commands and verifier stages run with your shell and
  your permissions.** loopy does not sandbox them. Registering an agent is
  exactly as dangerous as running that command yourself; review
  `.loopy/agents.json` and `.loopy/config.json` in repositories you did not
  create, before running any loop.
- **Verifier output is untrusted input to the agent.** It is embedded in the
  next iteration's prompt. Template variables (`{prompt}`, `{prompt_file}`,
  `{worktree}`, `{loop_id}`, `{goal}`, `{iteration}`) are always
  shell-quoted on expansion so verifier output cannot inject into the agent
  command line — but the agent itself reads that text and may act on it.
  Treat a loop in a repository with untrusted contributors as running
  untrusted instructions through your agent.
- **loopy makes no network calls and needs no credentials.** Agents you
  register may do both, under their own configuration.
- **Worktree isolation is a safety rail, not a sandbox.** Agents work in
  `.loopy/worktrees/<loop-id>/`, but nothing prevents a misbehaving agent
  process from writing elsewhere; budgets, forbidden paths, and review-gated
  acceptance limit blast radius, not capability.

## Supply chain

Release binaries are built by GitHub Actions from the tagged commit with
`CGO_ENABLED=0`, checksummed in `SHA256SUMS`, and published with build
provenance attestations (`gh attestation verify` works against them).
The module has one direct dependency (Bubble Tea v2).
