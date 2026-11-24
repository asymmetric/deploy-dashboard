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
      packages = forAll (pkgs: {
        default = pkgs.buildGoModule {
          pname = "deploy-dashboard";
          version = self.shortRev or self.dirtyShortRev or "dev";
          src = self;
          # stdlib only, no go.sum
          vendorHash = null;
          meta.mainProgram = "deploy-dashboard";
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
            cominPort = lib.mkOption {
              type = lib.types.port;
              default = 4243;
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
                  ++ lib.optional (cfg.pinsAddress != null) "-pins-listen=${hostPort cfg.pinsAddress cfg.pinsPort}"
                  # %d is the credentials directory
                  ++ lib.optional (cfg.pinsTokenFile != null) "-pins-token-file=%d/pins-token"
                  ++ lib.optional (cfg.ntfyTopicUrl != null) "-ntfy-topic-url=${cfg.ntfyTopicUrl}"
                );
                # DynamicUser can't read the token file directly
                LoadCredential = lib.optional (cfg.pinsTokenFile != null) "pins-token:${cfg.pinsTokenFile}";
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
