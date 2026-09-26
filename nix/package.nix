{
  lib,
  buildGoModule,
  go_1_26,
  buildNpmPackage,
  nodejs_24,
  pkg-config,
  wrapGAppsHook3,
  copyDesktopItems,
  makeDesktopItem,
  gtk3,
  webkitgtk_4_1,
  glib-networking,
  libayatana-appindicator,
  nss,
}:

let
  version = (lib.importJSON ../wails.json).info.productVersion;

  frontend = buildNpmPackage {
    pname = "zen-frontend";
    inherit version;

    src = ../frontend;
    nodejs = nodejs_24;
    npmDepsHash = "sha256-4yfSedOJeWqrHTtOOQcKszKG+v54iiWTYA72Ok7Z8Pg=";
    # react-popper, pulled in by Blueprint, only accepts React up to 18. A plain
    # npm ci warns about it online, but in the sandbox it tries to fetch react's
    # package metadata to re-resolve the peer and fails with ENOTCACHED.
    npmFlags = [ "--legacy-peer-deps" ];

    installPhase = ''
      runHook preInstall
      cp -r dist $out
      runHook postInstall
    '';
  };
in
# go.mod pins go 1.26.1, and nixpkgs sets GOTOOLCHAIN=local with no network to
# auto-download a newer toolchain - pin the compiler rather than rely on the
# default go being recent enough (it may not be for overlay consumers).
(buildGoModule.override { go = go_1_26; }) {
  pname = "zen";
  inherit version;

  src = lib.cleanSource ../.;
  # Hash the module cache rather than `go mod vendor`'s output: vendor/ holds
  # only the imported packages, so a new import from a module already in
  # go.mod would change the hash without touching go.mod or go.sum, and the
  # Nix workflow would not catch it.
  proxyVendor = true;
  vendorHash = "sha256-u5vh0HijXcSx1ah+tyMh5ndsiCHXNvszCY72HTjTbxk=";

  nativeBuildInputs = [
    pkg-config
    wrapGAppsHook3
    copyDesktopItems
  ];

  buildInputs = [
    gtk3
    webkitgtk_4_1
    glib-networking
  ];

  # The tag set a production `wails build` would use: `desktop` and `production`
  # come from the wails CLI itself, `prod` and `webkit2_41` from the repo's build tasks.
  tags = [
    "desktop"
    "production"
    "prod"
    "webkit2_41"
  ];

  ldflags = [
    "-s"
    "-w"
    "-X github.com/irbis-sh/zen-desktop/internal/config.Version=v${version}"
    # Self-updating cannot work on a read-only store path.
    "-X github.com/irbis-sh/zen-desktop/internal/selfupdate.NoSelfUpdate=true"
    # InstanceID is deliberately not set: the fixed fallback UUID in
    # internal/constants keeps the single-instance lock working and the build reproducible.
  ];

  subPackages = [ "." ];

  preBuild = ''
    rm -rf frontend/dist
    cp -r ${frontend} frontend/dist
  '';

  postInstall = ''
    mv $out/bin/zen-desktop $out/bin/zen
    install -Dm644 assets/logo.svg \
      $out/share/icons/hicolor/scalable/apps/zen-adblocker.svg
  '';

  # The entry name and the zen-adblocker icon name must stay in sync with
  # install.sh and internal/autostart/autostart_linux.go.
  desktopItems = [
    (makeDesktopItem {
      name = "zen-adblocker";
      desktopName = "Zen";
      genericName = "Ad Blocker";
      comment = "System-wide ad-blocker and privacy guard";
      exec = "zen";
      icon = "zen-adblocker";
      startupWMClass = "zen";
      categories = [
        "Network"
        "Security"
        "Utility"
      ];
      keywords = [
        "adblock"
        "privacy"
        "proxy"
      ];
    })
  ];

  preFixup = ''
    gappsWrapperArgs+=(
      # The tray support dlopens libayatana-appindicator3.so.1 (internal/systray/systray_linux.c).
      --prefix LD_LIBRARY_PATH : ${lib.makeLibraryPath [ libayatana-appindicator ]}
      # certutil is required for installing the root CA into NSS databases,
      # the only trust path available on systems without an FHS trust store.
      --prefix PATH : ${lib.makeBinPath [ nss.tools ]}
      # Autostart entries must launch the wrapper, not the hidden .zen-wrapped
      # binary that os.Executable reports (see internal/autostart). A bare
      # command rather than $out/bin/zen: the entry is written once and never
      # refreshed, and a store path stops existing after an upgrade and a
      # garbage collection. PATH resolution is what the desktop item above
      # relies on as well.
      --set-default ZEN_EXEC_PATH zen
    )
  '';

  meta = {
    description = "Simple, free and efficient ad-blocker and privacy guard";
    homepage = "https://irbis.sh/zen";
    changelog = "https://github.com/irbis-sh/zen-desktop/blob/master/CHANGELOG.md";
    license = lib.licenses.mit;
    mainProgram = "zen";
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
  };
}
