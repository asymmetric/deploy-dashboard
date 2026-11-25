{
  description = "Deployment dashboard for comin/niks3 pull-based NixOS deployments";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" ];
      forAll = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      packages = forAll (
        pkgs:
        let
          build =
            pname: attrs:
            pkgs.buildGoModule (
              {
                inherit pname;
                version = self.shortRev or self.dirtyShortRev or "dev";
                src = self;
                # stdlib only, no go.sum
                vendorHash = null;
                subPackages = [ "cmd/${pname}" ];
                meta.mainProgram = pname;
              }
              // attrs
            );
        in
        {
          default = build "deploy-dashboard" { };
          # fake backends for `nix run .#demo`; tests ran with default
          fake-backends = build "fake-backends" { doCheck = false; };
        }
      );

      # The dashboard against fake backends (internal/fake), on
      # http://127.0.0.1:8090 or $LISTEN. Extra arguments go to the dashboard.
      apps = forAll (pkgs: {
        demo = {
          type = "app";
          program = pkgs.lib.getExe (
            pkgs.writeShellApplication {
              name = "deploy-dashboard-demo";
              runtimeInputs = [
                self.packages.${pkgs.stdenv.hostPlatform.system}.default
                self.packages.${pkgs.stdenv.hostPlatform.system}.fake-backends
                pkgs.curl
              ];
              text = ''
                addr=127.0.0.1:''${FAKE_PORT:-8095}
                dir=$(mktemp -d)
                fake-backends -listen "$addr" -token-file "$dir/nixbot-token" &
                fake=$!
                trap 'kill "$fake" 2>/dev/null; rm -rf "$dir"' EXIT
                for _ in $(seq 50); do
                  curl -fsS -o /dev/null "http://$addr/repos/" 2>/dev/null && break
                  sleep 0.1
                done
                read -ra flags <<< "$(fake-backends -listen "$addr" -token-file "$dir/nixbot-token" -print-flags)"
                # comin scrapes go through the fakes; loopback is never proxied
                export HTTP_PROXY="http://$addr" http_proxy="http://$addr"
                unset NO_PROXY no_proxy
                deploy-dashboard "''${flags[@]}" -state-file="$dir/state.json" -interval=5s \
                  -listen="''${LISTEN:-127.0.0.1:8090}" "$@"
              '';
            }
          );
        };
      });

      devShells = forAll (pkgs: {
        default = pkgs.mkShell {
          packages = [
            pkgs.go
            pkgs.gopls
          ];
        };
      });

      nixosModules.default =
        {
          config,
          lib,
          pkgs,
          ...
        }:
        let
          cfg = config.services.deploy-dashboard;
          # IPv6 addresses need brackets in host:port
          hostPort = address: port: "${if lib.hasInfix ":" address then "[${address}]" else address}:${toString port}";
        in
        {
          options.services.deploy-dashboard = {
            enable = lib.mkEnableOption "deployment dashboard";
            package = lib.mkOption {
              type = lib.types.package;
              default = self.packages.${pkgs.stdenv.hostPlatform.system}.default;
            };
            address = lib.mkOption {
              type = lib.types.str;
              default = "127.0.0.1";
              description = "Address to listen on.";
            };
            port = lib.mkOption {
              type = lib.types.port;
              default = 8090;
              description = "Port to listen on.";
            };
            hosts = lib.mkOption {
              type = lib.types.listOf lib.types.str;
              description = "gitops hosts to show; pass the dotfiles flake's gitopsHosts.";
            };
            niks3Url = lib.mkOption {
              type = lib.types.str;
              description = "niks3 server URL; pins are read from <url>/pins/<host>.";
            };
            pinsAddress = lib.mkOption {
              type = lib.types.nullOr lib.types.str;
              default = null;
              example = "0.0.0.0";
              description = ''
                Address of the listener for POST /pins, where the pin effects
                post their records. null disables it. It serves only that
                endpoint, so it can be opened to CI without exposing the page.
              '';
            };
            pinsPort = lib.mkOption {
              type = lib.types.port;
              default = 8091;
              description = "Port of the POST /pins listener.";
            };
            pinsTokenFile = lib.mkOption {
              type = lib.types.nullOr lib.types.path;
              default = null;
              description = ''
                File with the bearer token POST /pins requires. null accepts
                any request that reaches the listener.
              '';
            };
            ntfyTopicUrl = lib.mkOption {
              type = lib.types.nullOr lib.types.str;
              default = null;
              description = "ntfy topic to poll for pin records, as an alternative to POST /pins.";
            };
            repoUrl = lib.mkOption {
              type = lib.types.nullOr lib.types.str;
              default = null;
              example = "https://github.com/owner/dotfiles";
              description = "Web URL of the config repo, for links to commits. null disables the links.";
            };
            cominPort = lib.mkOption {
              type = lib.types.port;
              default = 4243;
            };
            nixbot = {
              url = lib.mkOption {
                type = lib.types.nullOr lib.types.str;
                default = null;
                example = "https://nixbot.example.ts.net";
                description = "nixbot web URL, for the state of the newest build of each host. null disables it.";
              };
              repo = lib.mkOption {
                type = lib.types.str;
                example = "github/owner/dotfiles";
                description = "The config repo as it appears in nixbot's URLs: <forge>/<owner>/<name>.";
              };
              tokenFile = lib.mkOption {
                type = lib.types.nullOr lib.types.path;
                default = null;
                description = ''
                  File with a nixbot API token (created at <url>/settings), needed
                  for private repos. null sends no token.
                '';
              };
              branch = lib.mkOption {
                type = lib.types.str;
                default = "main";
                description = "Branch whose builds pin the hosts.";
              };
              effect = lib.mkOption {
                type = lib.types.str;
                default = "default.pin-{host}";
                description = "Name of each host's pin effect; {host} is replaced by the host.";
              };
            };
          };

          config = lib.mkIf cfg.enable {
            systemd.services.deploy-dashboard = {
              description = "Deployment dashboard";
              wantedBy = [ "multi-user.target" ];
              after = [ "network-online.target" ];
              wants = [ "network-online.target" ];
              serviceConfig = {
                ExecStart = lib.escapeShellArgs (
                  [
                    (lib.getExe cfg.package)
                    "-listen=${hostPort cfg.address cfg.port}"
                    "-hosts=${lib.concatStringsSep "," cfg.hosts}"
                    "-niks3-url=${cfg.niks3Url}"
                    "-comin-port=${toString cfg.cominPort}"
                    "-state-file=/var/lib/deploy-dashboard/state.json"
                  ]
                  ++ lib.optional (cfg.repoUrl != null) "-repo-url=${cfg.repoUrl}"
                  ++ lib.optional (cfg.pinsAddress != null) "-pins-listen=${hostPort cfg.pinsAddress cfg.pinsPort}"
                  # %d is the credentials directory
                  ++ lib.optional (cfg.pinsTokenFile != null) "-pins-token-file=%d/pins-token"
                  ++ lib.optional (cfg.ntfyTopicUrl != null) "-ntfy-topic-url=${cfg.ntfyTopicUrl}"
                  ++ lib.optionals (cfg.nixbot.url != null) (
                    [
                      "-nixbot-url=${cfg.nixbot.url}"
                      "-nixbot-repo=${cfg.nixbot.repo}"
                      "-nixbot-branch=${cfg.nixbot.branch}"
                      "-nixbot-effect=${cfg.nixbot.effect}"
                    ]
                    ++ lib.optional (cfg.nixbot.tokenFile != null) "-nixbot-token-file=%d/nixbot-token"
                  )
                );
                # DynamicUser can't read the token files directly
                LoadCredential =
                  lib.optional (cfg.pinsTokenFile != null) "pins-token:${cfg.pinsTokenFile}"
                  ++ lib.optional (cfg.nixbot.url != null && cfg.nixbot.tokenFile != null) "nixbot-token:${cfg.nixbot.tokenFile}";
                DynamicUser = true;
                StateDirectory = "deploy-dashboard";
                Restart = "on-failure";
                NoNewPrivileges = true;
                ProtectSystem = "strict";
                ProtectHome = true;
                PrivateTmp = true;
                PrivateDevices = true;
                RestrictAddressFamilies = [
                  "AF_INET"
                  "AF_INET6"
                ];
              };
            };
          };
        };
    };
}
