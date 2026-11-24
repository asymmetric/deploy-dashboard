# deploy-dashboard

> [!WARNING]
> This is alpha-quality slopware. It was written mostly by an LLM, for one person's homelab, and has had little review. Expect rough edges and breaking changes.

A web page that shows, for each NixOS host deployed pull-style by [comin](https://github.com/nlewo/comin) from [niks3](https://github.com/Mic92/niks3) pins, whether it runs what CI pinned for it, and if not, where the pipeline stopped.

The pipeline it assumes:

1. CI ([nixbot](https://github.com/Mic92/nixbot) effects) builds each host's toplevel on `main`, pushes it to niks3, and pins it as `pins/<host>`.
2. comin on each host polls its pin, substitutes the closure and switches to it.

Each host gets one of these states: `in sync`, `deploying`, `pinned, not yet deployed`, `deploy failing`, `pinned by hand` (the pin differs from CI's last pin, e.g. a rollback), `running path unknown`, `offline` and `offline, pin waiting` (the exporter is unreachable, so the last status seen is shown; normal for laptops), `unreachable` (never reached), `pin unknown`.

Planned: PR status and per-host build results from GitHub check runs.

## Usage

### Run locally

On a machine that can reach niks3 and every host's comin exporter:

```sh
nix develop
go run ./cmd/deploy-dashboard \
  -hosts=host1,host2 \
  -niks3-url=http://niks3.example.ts.net:5751 \
  -pins-listen=0.0.0.0:8091
```

Then open http://127.0.0.1:8090. Build the binary with `nix build`.

Flags:

| flag | default | |
|---|---|---|
| `-hosts` | (required) | comma-separated hosts, as reachable names for the exporter |
| `-niks3-url` | (required) | niks3 server; pins are read from `<url>/pins/<host>` |
| `-listen` | `127.0.0.1:8090` | HTTP listen address of the dashboard |
| `-pins-listen` | (off) | HTTP listen address for `POST /pins`, where the pin effects post their records |
| `-pins-token-file` | (none) | file with the bearer token `POST /pins` requires |
| `-ntfy-topic-url` | (off) | ntfy topic to poll for pin records instead |
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
    pinsAddress = "0.0.0.0"; # POST /pins on pinsPort, 8091
    # pinsTokenFile = "/run/secrets/deploy-dashboard-pins-token";
    # address = "127.0.0.1"; port = 8090; cominPort = 4243;
  };
}
```

The service runs as a `DynamicUser` and keeps its state in `/var/lib/deploy-dashboard/state.json`. By default the page listens on loopback only, so put a reverse proxy in front of it, e.g. at `config.services.deploy-dashboard.port`. `POST /pins` has its own listener, so it can be opened to CI (firewall, tailnet policy) without exposing the page.

### Prerequisites

- comin's exporter must be reachable from the dashboard host on each host's `-comin-port`.
- comin must export the deployed toplevel as the `out_path` label of `comin_deployment_info`. Upstream comin doesn't. With niks3, its `commit_id` label is always empty. Without the label, hosts show `running path unknown`, and only failures (`comin_last_deployment_failed`) are detected.
- niks3's read proxy must serve `pins/<name>`.
- Each pin effect must post its record after `niks3 push --pin`:

  ```sh
  curl -fsS --retry 3 --retry-all-errors \
    -d '{"host":"<host>","commit":"<rev>","toplevel":"<store path>"}' \
    http://dashboard.example.ts.net:8091/pins
  ```

  Add `-H "Authorization: Bearer $token"` if `-pins-token-file` is set. Records for hosts not in `-hosts`, or whose toplevel isn't a store path, are rejected with 400. Alternatively, publish the same JSON to an ntfy topic and pass `-ntfy-topic-url`. Without these records, the dashboard can't show which commit a pin came from, or detect a pin that was moved by hand.

### Development

```sh
go test ./...
gopls check -severity=hint <file>.go
```
