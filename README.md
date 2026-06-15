<div align="center">

<p>
  <picture>
    <img src="https://github.com/irbis-sh/zen-desktop/blob/master/assets/appicon.png?raw=true" alt="Zen's Blue Shield Logo" width="150" />
  </picture>
</p>

<h3>
  Zen: Your Comprehensive Ad-Blocker and Privacy Guard
</h3>

<blockquote>
There is, simply, no way, to ignore privacy. Because a citizenry’s freedoms are interdependent, to surrender your own privacy is really to surrender everyone’s.

Edward Snowden, Permanent Record
</blockquote>

![GitHub License](https://img.shields.io/github/license/irbis-sh/zen-desktop)
![GitHub release](https://img.shields.io/github/v/release/irbis-sh/zen-desktop)
![GitHub download counter](https://img.shields.io/github/downloads/irbis-sh/zen-desktop/total)

[**Website**](https://irbis.sh/zen) • [**Mastodon**](https://mastodon.social/@irbis_sh) • [**Bluesky**](https://bsky.app/profile/irbis.sh) • [**Discord**](https://discord.gg/zGeQVatAUm) • [**Documentation**](https://docs.irbis.sh/docs/zen/)

</div>

Zen is an open-source system-wide ad-blocker and privacy guard for Windows, macOS, and Linux. It works by setting up a proxy that intercepts HTTP requests from all applications, and blocks those serving ads, tracking scripts that monitor your behavior, malware, and other unwanted content. By operating at the system level, Zen can protect against threats that browser extensions cannot, such as trackers embedded in desktop applications and operating system components. Zen comes with many pre-installed filters, but also allows you to easily add hosts files and EasyList-style filters, enabling you to tailor your protection to your specific needs.

## Downloads

During the first run, Zen will prompt you to install a root certificate. This is required for Zen to be able to intercept and modify HTTPS requests. This certificate is generated locally and never leaves your device. For details on how this works and the steps we take to secure it, see our [security architecture](/docs/internal/security-architecture.md).

### Windows

- x64: [💾 Installer](https://github.com/irbis-sh/zen-desktop/releases/latest/download/Zen-amd64-installer.exe) | [📦 Portable](https://github.com/irbis-sh/zen-desktop/releases/latest/download/Zen_windows_amd64.zip)
- ARM64: [💾 Installer](https://github.com/irbis-sh/zen-desktop/releases/latest/download/Zen-arm64-installer.exe) | [📦 Portable](https://github.com/irbis-sh/zen-desktop/releases/latest/download/Zen_windows_arm64.zip)

Unsure which version to download? Click on 'Start' and type 'View processor info'. The 'System type' field under 'Device specifications' will tell you which one you need.

#### Winget

Zen is available via [Winget (Windows Package Manager)](https://github.com/microsoft/winget-pkgs/tree/master/manifests/z/ZenPrivacy). To install, run:

```bash
winget install ZenPrivacy.ZenDesktop
```

### macOS

- x64 (Intel): [💾 Installer](https://github.com/irbis-sh/zen-desktop/releases/latest/download/Zen-amd64.dmg) | [📦 Portable](https://github.com/irbis-sh/zen-desktop/releases/latest/download/Zen_darwin_amd64.tar.gz)
- ARM64 (Apple Silicon): [💾 Installer](https://github.com/irbis-sh/zen-desktop/releases/latest/download/Zen-arm64.dmg) | [📦 Portable](https://github.com/irbis-sh/zen-desktop/releases/latest/download/Zen_darwin_arm64.tar.gz)

Unsure which version to download? Learn at [Apple's website](https://support.apple.com/en-us/HT211814).

#### 🍺 Homebrew

Zen is available via [Homebrew](https://formulae.brew.sh/cask/zen-privacy) for both Intel and Apple Silicon. To install it, run:

```bash
brew install --cask zen-privacy
```

### Linux

- AUR: [👾 zen-adblocker-bin](https://aur.archlinux.org/packages/zen-adblocker-bin)
- x64: [📦 Portable](https://github.com/irbis-sh/zen-desktop/releases/latest/download/Zen_linux_amd64.tar.gz)
- ARM64: [📦 Portable](https://github.com/irbis-sh/zen-desktop/releases/latest/download/Zen_linux_arm64.tar.gz)

On Linux, automatic proxy configuration is currently only supported on GNOME- and KDE-based desktop environments.

## Screenshots

<table>
  <thead>
    <tr>
        <th>Request history</th>
        <th>Filter list manager</th>
    </tr>
  </thead>
  <tbody>
    <tr>
      <td>
        Request history shows all requests blocked by Zen. Each request can be inspected to see which filter and rule blocked it.
      </td>
      <td>
        Zen comes with many pre-installed filters. You can also add your own by providing a URL to a hosts file or an EasyList-style filter.
      </td>
    </tr>
    <tr>
      <td align="center" valign="top">
        <picture>
          <source media="(prefers-color-scheme: dark)" srcset="assets/screenshots/main-dark.png">
          <source media="(prefers-color-scheme: light)" srcset="assets/screenshots/main-light.png">
          <img alt="Screenshot of Zen's Home screen showing blocked network requests. One entry details a blocked request to a Marketo tracking script from politico.com, with several other advertising and analytics domains listed below. Navigation tabs and a Donate button appear at the top, and a blue Stop button is visible at the bottom." src="assets/screenshots/main-light.png">
        </picture>
      </td>
      <td align="center" valign="top">
        <picture>
          <source media="(prefers-color-scheme: dark)" srcset="assets/screenshots/regional-dark.png">
          <source media="(prefers-color-scheme: light)" srcset="assets/screenshots/regional-light.png">
          <img alt="Screenshot of Zen's Filter lists screen showing a regional category with multiple ad-block filter lists from different countries. Each list includes a name, a source URL, and toggle switches indicating whether it is enabled." src="assets/screenshots/regional-light.png">
        </picture>
      </td>
    </tr>
  </tbody>
</table>

## Development

Follow the [getting started guide](docs/internal/index.md#getting-started) to begin working on Zen development. If you have any questions, feel free to ask in the [Discussions](https://github.com/irbis-sh/zen-desktop/discussions/categories/q-a).

## Contributing

Zen needs your help! You can report bugs, suggest and implement features, improve the codebase, or help translate Zen into your language. Please refer to the [Contributing Guidelines](CONTRIBUTING.md) for more information.

## Special Thanks

Zen exists thanks to the support of many incredible people and organizations, including:

- Our contributors
  <a href="https://github.com/irbis-sh/zen-desktop/graphs/contributors">
  <img src="https://opencollective.com/zen-privacy/contributors.svg?width=890&button=false" alt="Avatars of all GitHub contributors to Zen" />
  </a>

- Our sponsors
  <a href="https://opencollective.com/zen-privacy#backers" target="_blank" rel="noreferrer noopener">
  <img src="https://opencollective.com/zen-privacy/backers.svg?width=890&button=false" alt="Avatars of all backers of Zen on Open Collective" />
  </a>

- [SignPath](https://signpath.io) and [SignPath Foundation](https://signpath.org/), who generously provide a free Windows certificate and code signing

  <a href="https://signpath.io" target="_blank" rel="noreferrer noopener">
  <img src="./assets/signpath-logo.png" width="260" />
  </a>

## License

This project is licensed under the [MIT License](https://github.com/irbis-sh/zen-desktop/blob/master/LICENSE). Some code and assets included with Zen are licensed under different terms. For more information, see the [COPYING](https://github.com/irbis-sh/zen-desktop/blob/master/COPYING.md) file.
