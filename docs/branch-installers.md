# Branch Installers

The install commands shown in the UI are currently pinned to the `alpha` branch.

Current branch-backed installer scripts:

- Connector: `https://raw.githubusercontent.com/vairabarath/zero-trust/alpha/scripts/setup.sh`
- Agent: `https://raw.githubusercontent.com/vairabarath/zero-trust/alpha/scripts/agent-setup.sh`
- Client: `https://raw.githubusercontent.com/vairabarath/zero-trust/alpha/scripts/client-install-release.sh`

Important behavior:

- The `curl` command downloads the installer script from `alpha`.
- The connector and agent installer scripts also download their systemd unit files from `alpha`.
- The installer binaries are still fetched from GitHub Releases using `releases/latest`.
- The agent installer now creates the `zero-trust-agent` system user/group automatically before starting `agent.service`.

That means:

- installer logic and unit-file changes come from the `alpha` branch
- binary contents come from the latest published release unless you build locally

If you need a full branch-based test, build the binary from the branch and install it locally instead of relying on `releases/latest`.
