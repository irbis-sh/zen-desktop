# Zen Privacy Policy

Last updated: April 19, 2026

This privacy policy applies to Zen – a free, open-source desktop ad-blocker and privacy guard.

**Your data is your business. We keep it that way.**

---

## No tracking, no data collection, no exceptions

We do **not** collect, store, or share any personal information – we don't want to, and we never will.

Zen runs entirely on your device. It doesn't "call home", doesn't log what you browse, and doesn't send any analytics to us or to third parties.

**Your traffic stays yours. Full stop.**

---

## How Zen works – and what it stores

Zen blocks unwanted content by acting as a local proxy. To do this effectively, it installs a local root certificate (CA) so it can inspect encrypted (HTTPS) traffic. We know that sounds scary, but here's what you should know:

* The certificate is generated **locally on your device** – not by us.
* The private key **never leaves your device** and is stored with strict permissions.
* All traffic processing happens **on your device**, in real time.
* We never see, store, or analyze your browsing traffic.
* All of this is verifiable in the publicly available source code.

To power its filtering engine, Zen uses *filter lists* – public collections of rules that tell it what to block. Zen includes several built-in lists and lets you add your own. For performance, it caches these lists locally. The cache is stored in a temporary directory and can be deleted at any time.

Zen also stores debug logs to help with troubleshooting. These logs are saved locally, with hostnames redacted. They are never sent to us or to any third party, and you can delete them at any time. If you're curious, there's an "Export logs" button in the settings that opens the folder where the logs are stored.

---

## External services we rely on

While Zen itself is completely self-contained, we do use a few third-party services to support our infrastructure:

* **Cloudflare**: Serves update manifests (`update-manifests.zenprivacy.net`). We do not use Cloudflare Web Analytics or any other tracking services. Like most CDNs, Cloudflare may log metadata such as IP addresses for operational and security purposes. If you prefer to avoid even that, you can disable update checks in Zen's settings.
* **GitHub**: Hosts our source code repository, downloads, and app updates. When you download Zen or receive an update, you're receiving a file from GitHub's CDN.
* **Filter list providers**: Zen downloads public filter lists (like EasyList) to power its filtering engine. These lists are fetched anonymously and used only on your device. No browsing data is ever sent to these providers.

If you're curious or concerned, we encourage you to review the privacy policies of those services – but none of them receive any identifiable information from Zen.

---

## How we protect Zen

We take several steps to protect the integrity and trustworthiness of Zen – not just in how it's built, but in how it's governed.

* Zen is **open-source**, developed in public, and fully auditable. Anyone can inspect the source code, the build process, and the release history. Transparency is not a slogan – it's baked into how we work.
* All app builds are **reproducible** and **attested** using GitHub Actions' artifact attestation system.
* We enforce strict security on our GitHub organization, including mandatory code reviews.
* Updates are currently served via a private Cloudflare R2 bucket. While this is secure in practice, we're actively working on a verifiable, cryptographically signed update system.

Want to dive deeper? Read [Zen's Security Architecture on GitHub](https://github.com/irbis-sh/zen-desktop/blob/master/docs/explanation/security-architecture.md).

---

## Local control, always

Zen is designed so that **everything stays on your device**. You're in full control.

You can:

* Delete all logs and cache files at any time.
* Remove the installed certificate using the built-in **"Uninstall CA"** button in the settings, or manually via your system's certificate manager.
* Uninstall Zen completely, whenever you want.

---

## Questions? Feedback?

If you have a question, concern, or suggestion – privacy-related or not – email us at [contact@irbis.sh](mailto:contact@irbis.sh) or drop by our [Discord server](https://discord.gg/zGeQVatAUm).

Zen is built by a small, independent team that deeply cares about privacy. We're always happy to hear from you.
