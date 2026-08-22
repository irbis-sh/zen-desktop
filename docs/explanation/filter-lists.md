# Filter Lists

## Which lists get a "trusted" status

> [Originally discussed here](https://github.com/irbis-sh/zen-desktop/issues/147#issuecomment-2521317897)

1. This problem should be approached in a manner similar to the [principle of least privilege](https://en.wikipedia.org/wiki/Principle_of_least_privilege). This means that **only lists that use trusted scriptlets (and use scriptlets at all) should be granted a trusted status**.
2. We should **keep the number of trusted filter lists to a minimum**. I suggest setting a limit of 5 for now.
3. Trusted filter lists should either **be open-source and distributed via a repo-linked CDN (such as GitHub), or maintained by a trusted, community driven organization**.

Considering the lists currently included in our default configuration, I propose granting trusted status to the following two lists:
1. AdGuard Base filter
2. AdGuard Spyware filter
