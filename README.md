# deploy-dashboard

> [!WARNING]
> This is alpha-quality slopware. It was written mostly by an LLM, for one person's homelab, and has had little review. Expect rough edges and breaking changes.

A web page that shows, for each NixOS host deployed pull-style by [comin](https://github.com/nlewo/comin) from [niks3](https://github.com/Mic92/niks3) pins, whether it runs what CI pinned for it, and if not, where the pipeline stopped.

The pipeline it assumes:

1. CI ([nixbot](https://github.com/Mic92/nixbot) effects) builds each host's toplevel on `main`, pushes it to niks3, and pins it as `pins/<host>`.
2. comin on each host polls its pin, substitutes the closure and switches to it.

Each host gets one of these states: `in sync`, `deploying`, `pinned, not yet deployed`, `deploy failing`, `pinned by hand` (the pin differs from CI's last pin, e.g. a rollback), `running path unknown`, `unreachable`, `pin unknown`.

Planned: PR status and per-host build results from GitHub check runs.

## Usage

### Run locally

On a machine that can reach niks3, ntfy and every host's comin exporter:

```sh
nix develop
go run ./cmd/deploy-dashboard \
  -hosts=host1,host2 \
  -niks3-url=http://niks3.example.ts.net:5751 \
  -ntfy-topic-url=https://ntfy.example.com/pins
```

Then open http://127.0.0.1:8090. Build the binary with `nix build`.

Flags:

| flag | default | |
|---|---|---|
| `-hosts` | (required) | comma-separated hosts, as reachable names for the exporter |
| `-niks3-url` | (required) | niks3 server; pins are read from `<url>/pins/<host>` |
| `-ntfy-topic-url` | (required) | topic the pin effects publish `{host, commit, toplevel}` to |
| `-listen` | `127.0.0.1:8090` | HTTP listen address |
| `-comin-port` | `4243` | comin's Prometheus exporter port on each host |
| `-state-file` | `state.json` | where pin records and the ntfy read position are kept |
| `-interval` | `15s` | refresh interval |

`GET /healthz` returns `ok`.

### Deploy as a NixOS service

Add this flake as an input, then on the host that runs it:

```nix
{
  imports = [ inputs.deploy-dashboard.nixosModules.default ];

  services.deploy-dashboard = {
    enable = true;
    hosts = [ "host1" "host2" ];
    niks3Url = "http://niks3.example.ts.net:5751";
    ntfyTopicUrl = "https://ntfy.example.com/pins";
    # address = "127.0.0.1"; port = 8090; cominPort = 4243;
  };
}
```

The service runs as a `DynamicUser` and keeps its state in `/var/lib/deploy-dashboard/state.json`. By default it listens on loopback only, so put a reverse proxy in front of it, e.g. at `config.services.deploy-dashboard.port`.

### Prerequisites

- comin's exporter must be reachable from the dashboard host on each host's `-comin-port`.
- comin must export the deployed toplevel as the `out_path` label of `comin_deployment_info`. Upstream comin doesn't. With niks3, its `commit_id` label is always empty. Without the label, hosts show `running path unknown`, and only failures (`comin_last_deployment_failed`) are detected.
- niks3's read proxy must serve `pins/<name>`.
- Each pin effect must publish its record to the ntfy topic after `niks3 push --pin`:

  ```sh
  curl -fsS -d '{"host":"<host>","commit":"<rev>","toplevel":"<store path>"}' https://ntfy.example.com/pins
  ```

  Without these records, the dashboard can't show which commit a pin came from, or detect a pin that was moved by hand.

### Development

```sh
go test ./...
gopls check -severity=hint <file>.go
```
