# Zen's Security Architecture

## General considerations

Zen's security is of critical importance. Since it's designed to handle all HTTP/HTTPS egress traffic from a user's computer, any potential vulnerability could seriously compromise privacy and security. We therefore try our best to secure both the application itself and its surrounding infrastructure. This document outlines the specific security measures we take to protect the application and its users.

Zen is open-source and developed in public. One of the key strengths of open-source software is that it allows for public scrutiny and peer review. I personally appreciate the way Meredith Whittaker, the president of Signal, articulates this in the context of Signal: [YouTube link (14:34)](https://www.youtube.com/live/AyH7zoP-JOg?t=874s), and I believe her argument applies just as strongly to our project.

Security is always a work in progress, and we encourage **informed and good-faith** discussions and suggestions. In case you have an idea or expertise you'd like to share - reach out to us in public or in private and we'll be happy to discuss. Please note that this invitation does not apply to vulnerability reports. If you believe you've found a vulnerability in Zen, please follow the responsible disclosure process outlined in [SECURITY.md](/SECURITY.md).

## Application-level security

In addition to standard security practices - writing Zen in a memory-safe language, using static analysis tools, vetting project dependencies, and more - we take the following specific steps to ensure application security:

### Protection against CA compromise

To enable its content blocking and privacy protection features, Zen generates and installs a root CA certificate on the user's computer. We, the project developers, wish there were a less intrusive way to achieve this, but there unfortunately isn't one. That said, here's what we do to protect users against potential CA compromise:

- The CA public/private key pair is generated locally on the user's computer.
- The private key is never sent to any remote server.
- The private key is stored with [minimal permissions](/internal/certstore/diskcertstore.go) (`0600`) to reduce (though not eliminate) the risk of it being compromised by malicious processes running on the same computer.

You can verify these claims by reviewing the relevant code in [`diskcertstore.go`](/internal/certstore/diskcertstore.go).

We're currently exploring ways to encrypt the private key using system APIs (see [Electron's safeStorage API documentation](https://www.electronjs.org/docs/latest/api/safe-storage) for examples). If you have suggestions or would like to contribute to this effort, please let us know.

### No proxying for sensitive hostnames

Zen configures the system proxy using a [PAC script](https://developer.mozilla.org/en-US/docs/Web/HTTP/Guides/Proxy_servers_and_tunneling/Proxy_Auto-Configuration_PAC_file) (see [the implementation](/internal/sysproxy/pac.go)). It allows Zen to specify a list of hostnames traffic **to which** should not be proxied. There's a list common to all platforms, as well as platform-specific lists for Windows and macOS. The [common list](/internal/sysproxy/exclusions/common.txt) includes the following categories:

- Sensitive
- Login (auth gateways)
- Government and eGov websites
- Password managers
- Banks and financial institutions
- Payment processors
- Messaging services
- Digital infrastructure providers (e.g. AWS, Azure, Google Cloud)

We encourage users to suggest additional entries for this list - both within the current categories and potentially new ones.

Platform-specific lists include hosts identified by OS vendors as ones that should not be proxied. See [the Windows list](/internal/sysproxy/exclusions/windows.txt) and [the macOS list](/internal/sysproxy/exclusions/darwin.txt) for details.

## Infrastructure-level security

### GitHub repository

Zen's source code is hosted on GitHub. We apply the following security measures:

- Two-factor authentication (2FA) is required for all organization members.
- All pull requests must pass all tests and checks before being merged.
- Only project leads can create and modify release tags and releases.
- CI actions follow best practices, such as minimal privileges and pinned action versions.

### Immutable releases

Zen's releases are immutable. Once a release is published, it – and the associated Git tag – cannot be modified. This ensures that a potential attacker cannot silently inject malicious binaries into an existing release.

To verify that a release is immutable, check for the "Immutable" label below the title on the release page.

To learn more about immutable releases, see [GitHub's documentation](https://docs.github.com/en/code-security/supply-chain-security/understanding-your-software-supply-chain/immutable-releases).


### Artifact attestations

Zen's artifacts, including release assets listed in our GitHub README and on the [project website](https://irbis.sh) are built [via GitHub CI](/.github/workflows/build.yml). The CI uses [artifact attestations](https://docs.github.com/en/actions/security-for-github-actions/using-artifact-attestations/using-artifact-attestations-to-establish-provenance-for-builds) to provide cryptographically verifiable proof of their provenance and integrity. In particular, the attestations allow you to verify that:

1. The artifact was indeed built by GitHub CI.
2. The artifact was built from the source code at the listed commit.
3. The artifact was not tampered with after the build or release.

To verify an attestation, install the [GitHub CLI](https://cli.github.com/) and run:

```bash
gh attestation verify PATH/TO/YOUR/BUILD/ARTIFACT-BINARY -R irbis-sh/zen-desktop
```

Note: If you've downloaded an archived artifact, the attestation will verify only the archive itself - not the individual files inside it.

### Update Delivery (Work in Progress)

Zen updates are currently uploaded and served from a private Cloudflare R2 bucket. This approach was chosen to avoid vendor lock-in and to mitigate potential deplatforming risks associated with GitHub.

While this setup works reliably and we take care to safeguard access to the bucket, we recognize that serving updates from a private object storage bucket without cryptographic verification is not a sufficient guarantee of security or integrity.

To address this, we plan to adopt a more robust and verifiable update mechanism in the close future. Specifically, we're evaluating options such as:

- Signing all update binaries with a project-owned key,
- Verifying signatures at runtime before installing updates,
- Using a framework like The Update Framework (TUF) to protect against targeted attacks and key compromise.

As always, if you have experience in secure update delivery or would like to help us improve this system, please reach out - we welcome any kind of help and collaboration.
