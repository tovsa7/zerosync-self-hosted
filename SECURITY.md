# Security Policy

## Reporting a vulnerability

If you find a security issue in this server — signaling protocol, room registry, blob-relay handling, container build, or the public `auth.Validator` interface — please **do not** open a public issue.

**Use GitHub's private vulnerability reporting:**

👉 [Report a vulnerability](https://github.com/tovsa7/zerosync-self-hosted/security/advisories/new)

Or email `contact.zerosync@proton.me` with the subject line `[security] zerosync-self-hosted: <short summary>`.

You will receive an acknowledgement within **72 hours**. Confirmed issues are fixed privately, then released as a patch version with a public security advisory.

## In scope

- Signaling protocol logic (`signaling/`)
- Room registry / peer-state safety (`room/`)
- Blob-relay handling (`relay/`)
- Container build (`Dockerfile`, GitHub Actions workflow)
- The public `auth.Validator` interface boundary (`auth/`)

## Out of scope

- The pluggable license validator implementation that lives in the private `zerosync-enterprise` repository — report those directly to the maintainer at the email above.
- Self-hosted infrastructure misconfigurations not caused by the server's defaults.
- Findings that depend on operators disabling default rate limits, IP-connection caps, or read deadlines.

## Cryptographic findings

This server does **not** perform cryptographic operations on user content. End-to-end encryption (AES-256-GCM, HKDF-SHA-256, mutual peer auth) lives in the client SDK at [github.com/tovsa7/ZeroSync](https://github.com/tovsa7/ZeroSync). Crypto-related findings should be filed against that repository's [SECURITY.md](https://github.com/tovsa7/ZeroSync/blob/main/SECURITY.md).

## Safe-harbor

Good-faith security research is welcome. We will not pursue legal action against researchers who comply with this policy and do not exfiltrate user data or disrupt service availability for other operators.
