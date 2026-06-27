# Changelog

## v0.24.0

### What's New
* **Linux app starts again without `libayatana-appindicator3`**
  Since `v0.23.0`, Zen failed to launch on Linux systems that didn't have AppIndicator installed, quitting with a shared-library loading error. Zen now loads the system tray library at runtime instead of linking it at build time, so the app starts whether or not the library is present. If you ran into this after updating, see the troubleshooting guide: https://docs.irbis.sh/docs/zen/how-to/troubleshoot/#zen-wont-start-after-updating-to-v0230
* **`important` modifier**
  Zen now supports the `important` rule modifier. An important rule wins over regular exception rules, and only an important exception rule can cancel it. Thanks to @LucasM4r!
* **`ping` content-type modifier**
  Rules can now match hyperlink auditing (ping) requests with the `ping` content-type modifier. Thanks to @LucasM4r!
* **Noop modifiers**
  Zen now accepts noop modifiers (`_`) in rules and ignores them during parsing, which improves compatibility with filter lists. Thanks to @kamalovk!

### New Contributors
* @LucasM4r made their first contribution in https://github.com/irbis-sh/zen-desktop/pull/726

Thank you for using Zen!

**Full Changelog**: https://github.com/irbis-sh/zen-desktop/compare/v0.23.0...v0.24.0

## v0.23.0

### What's New

* **Linux system tray support**
  Zen now sits in the system tray on Linux in supported desktop environments. Closing the window minimizes Zen to the tray and keeps the proxy running instead of shutting it down. Thanks to @ohdonny!
* **こんにちは！**
  Zen's UI is now available in Japanese, thanks to @AI-Avalon!
* **WebSocket filtering**
  WebSocket traffic can now be matched and blocked. Additionally, Zen now recognizes the `$websocket` content type modifier. Thanks to @inflame-ue!
* **More reliable content-type matching**
  Content-type rules now match against the `Content-Type` response header, not only the `Sec-Fetch-Dest` request header. These rules now work more consistently, especially with non-browser apps. Thanks to @Mirwinli!
* **Separate PAC port setting**
  Settings > Advanced now has a field for the PAC proxy port. Useful for apps that do not respect system proxy settings automatically and need to be configured to use Zen manually.
* **Other improvements**
  Tightened the scope of generated certificates (thanks @inflame-ue!), fixed a resource leak when the proxy fails to start (thanks @Mirwinli!), improved filtering accuracy (thanks @Mirwinli!), and made other internal improvements to the filtering engine.

### New Contributors
* @Mirwinli made their first contribution in https://github.com/irbis-sh/zen-desktop/pull/700
* @AI-Avalon made their first contribution in https://github.com/irbis-sh/zen-desktop/pull/696
* @inflame-ue made their first contribution in https://github.com/irbis-sh/zen-desktop/pull/717

Thank you for using Zen!

**Full Changelog**: https://github.com/irbis-sh/zen-desktop/compare/v0.22.0...v0.23.0

## v0.22.0

### What's New

* **Regular-expression rule support**
  Zen now supports regexp rules. This improves compatibility with filter lists that use regexp patterns to match dynamic tracking and ad URLs.
* **Fixed allowlisting for hosts rules**
  The "Allow" button on "Blocked by Zen" pages now works correctly when the block was caused by a hosts-style rule such as `0.0.0.0 example.net`.
* **Fixed macOS service compatibility**
  HTTPS exclusions were updated to fix sign-in and calling issues with Apple Messages and FaceTime. Thanks to @rishiskhare for reporting the issue!
* **Removed a stale built-in filter lists**
  An outdated Polish anti-adblock list has been removed from the configuration. Thanks to @qorexdevs for the contribution!
* **UI polish**
  The header and filter lists now use cleaner divider styling for a more consistent look.

### New Contributors
* @qorexdevs made their first contribution in https://github.com/irbis-sh/zen-desktop/pull/682

Thank you for using Zen!

**Full changelog**: https://github.com/irbis-sh/zen-desktop/compare/v0.21.1...v0.22.0

## v0.21.1

### What's New

This is a hotfix for `0.21.0`, which had an incorrect version number in the file manifest.

**Full Changelog**: https://github.com/irbis-sh/zen-desktop/compare/v0.21.0...v0.21.1

## v0.21.0

### What's New
- **App allow- and block-listing**
  The new **"App routing"** setting gives you the ability to choose which apps should use Zen's proxy. It gives you the ability to exclude a developer tool which has trouble with proxying, a gaming app where you want maximum performance, or a browser with built-in ad-blocking.
- Other minor improvements.

**Full Changelog**: https://github.com/irbis-sh/zen-desktop/compare/v0.20.0...v0.21.0

## v0.20.0

### What's new
- **Process info in request logs**
  Request logs now show information about processes which initiated the request. This makes it easier to trace activity and understand which applications are responsible for specific traffic.
- **Merhaba!**
  Zen now speaks Turkish, thanks to @Wek1d! Want to contribute a new language or improve an existing one? Check out our contributing guidelines.
- More minor improvements.

### New contributors
- @Wek1d made their first contribution in https://github.com/ZenPrivacy/zen-desktop/pull/641

**Full changelog**: https://github.com/ZenPrivacy/zen-desktop/compare/v0.19.2...v0.20.0

## v0.19.2

### What's New
- __Faster request matching__
  Optimised a bottleneck in the request matching engine that accounted for 10-15% of overall CPU usage. Browsing should feel a little snappier across the board.
- __macOS network permissions__
  Improved compatibility with the "Require an administrator password to access system-wide settings" option in macOS. Zen now correctly elevates privileged commands instead of failing.
- __macOS login item naming__
  Fixed macOS login items displaying the developer name instead of the app name – thanks to @ganeshmshetty!

### New Contributors
- @ganeshmshetty made their first contribution in https://github.com/ZenPrivacy/zen-desktop/pull/627

Thank you for using Zen!

**Full Changelog**: https://github.com/ZenPrivacy/zen-desktop/compare/v0.19.1...v0.19.2

## v0.19.1

### What's New

This is a hotfix to v0.19.0.

- __Steam fix__
  This release fixes the "NO CONNECTION" issue in Steam.

Thank you for using Zen!

**Full Changelog**: https://github.com/ZenPrivacy/zen-desktop/compare/v0.19.0...v0.19.1

## v0.19.0

### What's New

This is a performance-focused update with many improvements across the stack.

- Zen's proxy now speaks **HTTP/2** in addition to HTTP/1.1, for both inbound and outbound traffic. Thanks to @brycewray for initiating the conversation on this.
- The rule matching engine is **2x faster on average**, with more than **200x improvement on long URLs** (1,000+ characters). This particularly affected Google Meet and Google Chat – expect much improved loading times on these services.
- Extended CSS rule application now optimizes `:has`, `:not`, and `:is` pseudo-class evaluation when it can be run natively. Thanks to @krystian3w for advice along the way.
- Other minor improvements and bug fixes.

### New Contributors
- @LinaKACI-pro made their first contribution in https://github.com/ZenPrivacy/zen-desktop/pull/603

Thank you for using Zen!

**Full Changelog**: https://github.com/ZenPrivacy/zen-desktop/compare/v0.18.1...v0.19.0

## v0.18.1

### What's New
- __Fixed self-updates__
  Self-updates were failing for some users due to a 20-second timeout that sometimes wasn't enough to download the update over GitHub's CDN. The timeout has been increased to fix this.

Thank you for using Zen!

**Full Changelog**: https://github.com/ZenPrivacy/zen-desktop/compare/v0.18.0...v0.18.1

## v0.18.0

### What's New
- __no-doomscroll__
  Zen now ships with no-doomscroll – a set of filter lists that remove infinite feeds from social media websites, letting you use them without getting pulled into endless scrolling. You can enable them under Filter lists / Digital wellbeing. Learn more on the [no-doomscroll homepage](https://github.com/ZenPrivacy/filter-lists/blob/master/no-doomscroll/readme.md).
- Other minor improvements.

Thank you for using Zen!

__Full Changelog__: https://github.com/ZenPrivacy/zen-desktop/compare/v0.17.0...v0.18.0

## v0.17.0

### What's New

- __Support for the `!#include` directive in filter lists__
  This improves compatibility with a wider range of filter lists, including regional ones.
- __`:style()` extended CSS pseudo-class support__
  Zen is now even better at hiding unwanted elements on webpages.
- __Improved cache behavior__
  Injected page assets are no longer cached by browsers, so pages update instantly when you change the configuration or turn Zen off.
- __Improved Linux support__
  Added partial support for XFCE – thanks to @xoxorwr!
- Other bug fixes and performance improvements.

### New Contributors

- @xoxorwr made their first contribution in https://github.com/ZenPrivacy/zen-desktop/pull/558

Thank you for using Zen!

__Full Changelog__: https://github.com/ZenPrivacy/zen-desktop/compare/v0.16.0...v0.17.0

## v0.16.0

### What's New
- __Performance improvements__
  We've optimized our filtering engine, with significant performance gains on long URLs.
- __Better UI__
  The application now includes a new logo and a new font, improving visual consistency across platforms, along with other minor UI refinements.
- __Better injection reliability__
  Improved handling of Content Security Policy (CSP) ensures that injections work more reliably across a wider range of sites. Thanks to @kasyap1234 for the contribution!
- __NSS trust store__
  The app now also installs the CA certificate into the NSS trust store. This notably improves the experience on Firefox – you're now less likely to encounter certificate errors on first launch. Thanks @donnykd for implementing this!
- __More reliable proxy on macOS__
  We've improved how the system proxy is configured on macOS, ensuring filtering remains active across all network interfaces. This also improves compatibility with various VPNs.
- Other minor improvements and bug fixes.

Thank you for using Zen!

**Full Changelog**: https://github.com/ZenPrivacy/zen-desktop/compare/v0.15.4...v0.16.0

## v0.15.4

### What's New
- __File downloads fix__
  Fixed an issue that caused downloaded files to become corrupted. Thanks to @rugabunda for reporting it.

Thank you for using Zen!

**Full Changelog**: https://github.com/ZenPrivacy/zen-desktop/compare/v0.15.3...v0.15.4

## v0.15.3

### What's New
- __Windows uninstaller fix__
  The uninstaller now automatically terminates any running instances of Zen, ensuring a complete removal.
- Other minor bug fixes.

Thank you for using Zen!

**Full Changelog**: https://github.com/ZenPrivacy/zen-desktop/compare/v0.15.2...v0.15.3

## v0.15.2

### What's New
- **Hosts rules fix**
  Host style rules are now working as intended.
- Other minor bugfixes.

Thank you for using Zen!

**Full Changelog**: https://github.com/ZenPrivacy/zen-desktop/compare/v0.15.1...v0.15.2

## v0.15.1

### What's New
This release fixes the "myRules is nil" error experienced during application startup. We apologize for the inconvenience.

**Full Changelog**: https://github.com/ZenPrivacy/zen-desktop/compare/v0.15.0...v0.15.1

## v0.15.0

### What's New
- __Salut!__
  Zen now speaks French, thanks to @Armitryx! Want to contribute a new language or improve an existing one? Check out our contributing guidelines.
- Bug fixes and minor improvements.

Thank you for using Zen!

**Full Changelog**: https://github.com/ZenPrivacy/zen-desktop/compare/v0.14.0...v0.15.0

## v0.14.0

### What's New

- **你好！**
  Zen now speaks Traditional Chinese, thanks to @lynda0214! Want to contribute a new language or improve an existing one? Check out our contributing guidelines.
- Minor stability improvements and bug fixes.

### New Contributors

- @lynda0214 made their first contribution in https://github.com/ZenPrivacy/zen-desktop/pull/485

**Full Changelog**: https://github.com/ZenPrivacy/zen-desktop/compare/v0.13.1...v0.14.0

## v0.13.1

### What's New

- **Default update policy fix**
  The default update policy is now set to automatic, ensuring users automatically receive the latest updates by default.

Thank you for using Zen!

**Full Changelog**: https://github.com/ZenPrivacy/zen-desktop/compare/v0.13.0...v0.13.1

## v0.13.0

### What's New

- **Background update checks**
  Zen now periodically checks for updates in the background, ensuring you get the latest updates faster.
- **Interactive onboarding**
  A new smooth onboarding experience walks you through the initial app setup. Thanks to @kamalovk!
- **Memory optimization**
  Rules are now stored more efficiently, reducing the overall RAM usage. Thank you @lzap for help with the improvements!
- **KDE system proxy support**
  Zen now properly sets system proxy settings on KDE, which improves integration with the desktop environment. Thanks to @donnykd for implementing this feature!
- **Ciao!**
  Zen now speaks Italian, thanks to @davide-damico! Want to contribute a new language or improve an existing one? Check out our contributing guidelines.
- **More MITM exclusions**
  Thanks to @Speedauge, Zen now excludes more traffic from sensitive websites from proxying, improving the overall security of your system.
- **Extended CSS improvements**
  We're continuing to work on our extended CSS engine, bringing more stability and features.
- More minor improvements and bug fixes.

### New Contributors

- @davide-damico made their first contribution in https://github.com/ZenPrivacy/zen-desktop/pull/454
- @lzap made their first contribution in https://github.com/ZenPrivacy/zen-desktop/pull/458
- @Speedauge made their first contribution in https://github.com/ZenPrivacy/zen-desktop/pull/481

Thank you for using Zen!

**Full Changelog**: https://github.com/ZenPrivacy/zen-desktop/compare/v0.12.0...v0.13.0

## v0.12.0

### What's New

- **Extended CSS**
  Zen now supports extended (procedural) CSS, enabling more powerful and comprehensive visual content blocking.
- **Block page**
  When a web page request is blocked, Zen now displays a dedicated block page, making it easier to review and unblock rules if needed.
- **CSP improvements**
  Fixed issues with Content Security Policy (CSP) modification and added support for patching inline styles, improving site compatibility while keeping everything secure.
- **macOS autostart fix**
  Resolved an issue where Zen's filtering wouldn't start automatically on macOS when launched at login.
- **Safer archive handling**
  Zen now uses Go's built-in security features to handle ZIP and TAR files more safely during updates, reducing the risk of path traversal issues. Thanks to @donnykd!
- Other bug fixes and small improvements.

Thank you for using Zen!

**Full Changelog**: https://github.com/ZenPrivacy/zen-desktop/compare/v0.11.3...v0.12.0

## v0.11.3

### What's New

- **Fixed "Phishing URL Blocklist" format**
  Resolved an issue with the **Phishing URL Blocklist** that caused overly aggressive request blocking. Zen should now behave correctly when this list is enabled.

Thank you for using Zen!

**Full Changelog**: https://github.com/ZenPrivacy/zen-desktop/compare/v0.11.2...v0.11.3

## v0.11.2

### What's New

- **Reduced memory consumption**
  Zen now uses about 10–15% less memory, thanks to work by @ilovelinabell.
- **你好！**
  Zen now speaks Chinese, thanks to @ccarstens! Want to contribute a new language or improve an existing one? Check out our contributing guidelines.
- **Start/stop button hotkey**
  You can now press **Space** to quickly start and stop the proxy. Thank you, @donnykd, for the contribution!
- **Single instance locking**
  Zen now keeps only a single instance of the application active, ensuring that it doesn't clutter your desktop. Thanks to @kasyap1234!
- **Windows binary signing**
  With the help of SignPath, Zen's Windows binaries are now signed with a certificate. Expect no more warnings during installation and fewer false antivirus flags.
- **Filter list buttons**
  You can now quickly copy and open filter lists in your browser. Thanks to @RustemMT for the contribution.
- Bug fixes and other small improvements.

### New Contributors

- @ccarstens made their first contribution in https://github.com/ZenPrivacy/zen-desktop/pull/373
- @donnykd made their first contribution in https://github.com/ZenPrivacy/zen-desktop/pull/372
- @ilovelinabell made their first contribution in https://github.com/ZenPrivacy/zen-desktop/pull/350
- @kasyap1234 made their first contribution in https://github.com/ZenPrivacy/zen-desktop/pull/381
- @BUTTER-BEAR made their first contribution in https://github.com/ZenPrivacy/zen-desktop/pull/389
- @RustemMT made their first contribution in https://github.com/ZenPrivacy/zen-desktop/pull/397

Thank you for using Zen!

**Full Changelog**: https://github.com/ZenPrivacy/zen-desktop/compare/v0.11.1...v0.11.2

## v0.11.1

### What's New

- **CSP injection fix**
  Fixed an issue with script injection on some websites using `unsafe-inline` in their Content Security Policy. Zen now correctly avoids using `nonce`-based injection when it's incompatible, restoring functionality on affected sites.

Thank you for using Zen!

**Full Changelog**: https://github.com/ZenPrivacy/zen-desktop/compare/v0.11.0...v0.11.1

## v0.11.0

### What's New

- **Clipboard sanitization**
  Some websites add tracking parameters (like utm_source and si) to URLs copied via their "Share" buttons. Zen now strips these trackers for your privacy. No more sneaky tracking when sharing content with your friends.
- **More reliable scriptlets injection**
  Zen can now inject scriptlets on more sites — including those with strict Content Security Policies (CSP) — ensuring our protections stay active without compromising your security.
- **Fixes to regional filter lists**
  Fixed a broken link to RU AdList and replaced the discontinued Icelandic filter list with its maintained Brave version.
- Bug fixes and other improvements.

Thank you for using Zen!

**Full Changelog**: https://github.com/ZenPrivacy/zen-desktop/compare/v0.10.1...v0.11.0

## v0.10.1

### What's New

- **Fixed Windows installer**:
  The Windows installer for the version 0.10.0 has unfortunately included the version of the app with its self-update capabilities disabled. If you're on Windows, not getting the prompt for this update, and missing the "Choose how updates are installed" option in the settings, please manually uninstall the app and download a newer version. We apologize for the inconvenience.
- **Proxy exclusions**:
  The list of proxy exclusions now includes more hosts. This fixes error with Apple Pay on macOS, the desktop ChatGPT app, and improves your security overall.
- **Homebrew**:
  The macOS app is now available on Homebrew. Go to our homepage, zenprivacy.net, to get the installation instructions.

### New Contributors

- @michaelthatsit made their first contribution in <https://github.com/ZenPrivacy/zen-desktop/pull/352>

Thank you for using Zen!

**Full Changelog**: <https://github.com/ZenPrivacy/zen-desktop/compare/v0.10.0...v0.10.1>

## v0.10.0

### What's New

- **Stronger content-blocking engine**

  - New rule modifiers: `jsonprune` and `remove-js-constant` enable detection-free YouTube ad-blocking.
  - New scriptlets: `prevent-setTimeout`, `prevent-setInterval`, `prevent-addEventListener`, `no-topics`, and `no-protected-audience`.
  - Improved filter list handling with on-disk caching and minor correctness/performance enhancements.

- **Fresh Light theme**
  Switch between dark and light modes to match your desktop or daylight.

- **Expanded language support**
  Сәлем! Hallo! Zen now speaks **Kazakh** and **German**. Want to contribute a new language or improve an existing one? Check out our contributing guidelines.

- **Enhanced security & supply chain hardening**

  - Automatic removal of Zen's root CA on Windows during uninstallation.
  - Build artifact attestation for improved supply-chain security.

- **UI & UX improvements**
  Donate button, a quick link to the changelog, disabled controls when the proxy is active, and more.

### New Contributors

- **@pulkitgarg04** – updated Hungarian filter list URL
- **@colinfrerichs** – config updates for v0.10.0

**Full Changelog**: [github.com/ZenPrivacy/zen-desktop/compare/v0.9.0...v0.10.0](https://github.com/ZenPrivacy/zen-desktop/compare/v0.9.0...v0.10.0)

## v0.9.0

### What's New

- **Multi-language support**: Zen now supports multiple languages, with more on the way. You can switch your preferred language in the settings. Huge thanks to @kamalovk for laying the groundwork for this feature.
- **Background self-updates**: Zen can now check for and apply updates automatically in the background at startup. You can enable this behavior in the settings.
- **Minimized startup**: When autostart is enabled on Windows, Zen now launches minimized to the system tray - keeping things quiet until you need them. Thanks to @Zanphar for the suggestion.
- **Scriptlet enhancements**: Numerous improvements to scriptlets, including new additions and stability upgrades to existing ones.
- **Internal filtering engine improvements**: The filtering engine now supports precise exceptions, which allows for more unwanted content to be blocked.
- **Higher resolution icons on Windows**: Zen now features sharper, high-resolution icons on Windows, thanks to @TobseF.
- **ARM64 builds for Linux**: Native ARM64 builds are now available for Linux users.
- **System proxy configuration via PAC**: Zen now configures the system proxy using a PAC file, resolving issues with networking in built-in Windows apps and improving overall security.
- **Join our Discord community**: We've launched a Discord server! Come say hi, share tips, and stay up to date with the latest on Zen: <https://discord.gg/jSzEwby7JY>. You'll also find the link on our website: <https://zenprivacy.net>.

### New Contributors

- @kamalovk made their first contribution: <https://github.com/ZenPrivacy/zen-desktop/pull/269>
- @TobseF made their first contribution: <https://github.com/ZenPrivacy/zen-desktop/pull/267>

**Full Changelog**: <https://github.com/ZenPrivacy/zen-desktop/compare/v0.8.0...v0.9.0>

## v0.8.0

### What's New

- **Performance Improvements**: We rewrote our proxy so that it no longer waits for the entire response before starting to pass data to the browser. Expect 1.5–2× improvements in page download times.
- Minor enhancements to content blocking and privacy preservation.

Thank you for using Zen!

**Full Changelog**: <https://github.com/ZenPrivacy/zen-desktop/compare/v0.7.2...v0.8.0>

## v0.7.2

### What's New

- **Character Encoding Fix**: Improved character encoding detection to handle websites with non-standard encodings more gracefully. Many thanks to @2372281891 for reporting the issue.

Thank you for using Zen!

**Full Changelog**: <https://github.com/ZenPrivacy/zen-desktop/compare/v0.7.1...v0.7.2>

## v0.7.1

### What's New

- **Navigator API Bug Fix**: Resolved a critical issue that impacted the stability of websites using the Navigator API.

Thank you for using Zen!

**Full Changelog**: <https://github.com/ZenPrivacy/zen-desktop/compare/v0.7.0...v0.7.1>

## v0.7.0

### What's New

- **Cosmetic Filtering**: Annoying and intrusive elements on webpages are now automatically blocked for a cleaner browsing experience.
- **JavaScript Rule Injection**: JS rules expand on scriptlets and offer advanced ad-blocking and privacy-preserving capabilities in the most complex cases.
- **Windows System Tray Icon Stability**: Resolved an issue where the tray icon could become unresponsive after prolonged use on Windows.
- Various stability improvements and bug fixes.

Happy 2025 and thank you for using Zen!

**Full Changelog**: <https://github.com/ZenPrivacy/zen-desktop/compare/v0.6.1...v0.7.0>

## v0.6.0

### What's New

- **Scriptlets**: Introducing scriptlets—advanced ad-blocking tool designed to handle cases where regular filtering is insufficient.
  - **First-Party Self-Update**: We've completely rewritten our self-updating system for improved stability. Future macOS updates will now be delivered seamlessly without requiring a reinstallation of the app. Special thanks to @AitakattaSora for implementing this feature.
  - **Custom Filter List Backup**: Advanced users can now easily back up and restore their custom filter lists. Many thanks to @Noahnut for your contribution.
  - **Rules Editor**: A new tab in the app allows you to add custom filter rules directly inside the app.
  - **Export Application Logs**: Logs are now written to disk, making it easier for the development team to diagnose and resolve issues. Thank you to @AitakattaSora for implementing this feature.
  - **Improved Linux Support**: The app now starts without errors on non-GNOME systems. You can now manually configure the HTTP proxy on a per-app basis if needed. Thanks to @AitakattaSora for this enhancement.
  - **Improved Windows Support**: The app now shuts down gracefully and resets the system proxy during system shutdown, preventing internet disruptions at startup.
  - Various stability improvements and bug fixes.

Warning: On macOS, the app will not function properly after the update. Please visit our homepage, [zenprivacy.net](https://zenprivacy.net), to manually download the latest version. Future updates will be delivered seamlessly.

Thank you for using Zen!

**Full Changelog**: <https://github.com/ZenPrivacy/zen-desktop/compare/v0.5.0...v0.6.0>
