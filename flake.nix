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
            ntfyTopicUrl = lib.mkOption {
              type = lib.types.str;
              description = "ntfy topic URL the pin effects publish to.";
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
                ExecStart = lib.escapeShellArgs [
                  (lib.getExe cfg.package)
                  # IPv6 addresses need brackets in host:port
                  "-listen=${
                    if lib.hasInfix ":" cfg.address then "[${cfg.address}]" else cfg.address
                  }:${toString cfg.port}"
                  "-hosts=${lib.concatStringsSep "," cfg.hosts}"
                  "-niks3-url=${cfg.niks3Url}"
                  "-ntfy-topic-url=${cfg.ntfyTopicUrl}"
                  "-comin-port=${toString cfg.cominPort}"
                  "-state-file=/var/lib/deploy-dashboard/state.json"
                ];
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
