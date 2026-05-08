# https-exclusions

This repository contains hostnames that are excluded from proxying by Zen Personal and Enterprise. Hostnames may be excluded for:
- __Security__: sensitive personal data, financial infrastructure, government websites, and more
- __Compatibility__: hostnames that break when accessed through a proxy due to MITM issues or false-positive bot detection

The lists are:
- [`common.txt`](/common.txt) – excluded on all platforms
- [`windows.txt`](/windows.txt) – excluded on Windows
- [`darwin.txt`](/darwin.txt) – excluded on macOS

## Contributing

Contributions are welcome! To suggest a hostname for exclusion, please open an issue or submit a pull request with the hostname and a brief explanation of why it should be excluded.

## License

This repository is licensed under the [MIT License](/LICENSE).
