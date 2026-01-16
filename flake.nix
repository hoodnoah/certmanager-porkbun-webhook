{
  description = "Development shell for local k8s, Docker, Helm, Minikube for testing";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-24.05";
  };

  outputs = {self, nixpkgs}:
    let
      # list supported architectures
      supportedSystems = ["x86_64-linux" "aarch64-darwin"];

      # function to produce development shell for a given supported system
      mkDevShell = system:
        let 
          pkgs = import nixpkgs {system = system;};
        in
          pkgs.mkShell {
            buildInputs = [
              pkgs.minikube
              pkgs.kubectl
              pkgs.kubernetes-helm
              pkgs.git
              pkgs.just
              pkgs.go
            ];
         };

  # helper function to get nixpkgs for a given platform
  forAllSystems = systems: f: builtins.listToAttrs (map (system: {
    name = system;
    value = f system;
  }) systems);

  in {
    devShell = forAllSystems supportedSystems mkDevShell;
  };
}